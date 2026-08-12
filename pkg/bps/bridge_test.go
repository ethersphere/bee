// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	ma "github.com/multiformats/go-multiaddr"

	"github.com/ethersphere/bee/v2/pkg/bps"
	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	bpstesting "github.com/ethersphere/bee/v2/pkg/bps/testing"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// countingConnecter is a Connecter that always resolves to the same overlay and
// counts how many times it was asked to dial, which is how the mux tests prove
// that a second attach on a live topic reuses the session rather than opening
// a second one.
type countingConnecter struct {
	mu   sync.Mutex
	n    int
	addr *bzz.Address
}

func (c *countingConnecter) Connect(_ context.Context, _ []ma.Multiaddr) (*bzz.Address, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
	return c.addr, nil
}

func (c *countingConnecter) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// newBridge returns a broker, a bridge whose client service routes to it, the
// bridge's counting connecter and the underlay every attach is made with.
func newBridge(t *testing.T) (*bps.Service, *bps.Bridge, *countingConnecter, ma.Multiaddr) {
	t.Helper()

	broker, recorder, brokerAddr := newBroker(t, bps.Options{})

	client := bps.New(recorder, log.Noop, bps.Options{})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Fatal(err)
		}
	})

	underlay, err := ma.NewMultiaddr("/ip4/127.0.0.1/tcp/1634")
	if err != nil {
		t.Fatal(err)
	}
	conn := &countingConnecter{addr: &bzz.Address{
		Underlays: []ma.Multiaddr{underlay},
		Overlay:   brokerAddr,
	}}

	bridge := bps.NewBridge(client, conn, log.Noop)
	t.Cleanup(func() {
		if err := bridge.Close(); err != nil {
			t.Fatal(err)
		}
	})
	return broker, bridge, conn, underlay
}

// recvSink reads one message from an attachment's sink.
func recvSink(t *testing.T, a bps.Attachment) *soc.SOC {
	t.Helper()

	select {
	case s, ok := <-a.Messages():
		if !ok {
			t.Fatal("attachment closed before delivering a message")
		}
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a broadcast")
		return nil
	}
}

func TestBridgeMuxTwoSinks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, bridge, conn, underlay := newBridge(t)
	spec, _, msg := anchorCohort(t, topic(0x71), []byte("muxed"))

	pub, err := bridge.Attach(ctx, bps.AttachOptions{
		Peer:  underlay,
		Topic: swarm.NewAddress(spec.Topic),
		Spec:  spec,
		Owner: spec.Admin,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	sub, err := bridge.Attach(ctx, bps.AttachOptions{
		Peer:  underlay,
		Topic: swarm.NewAddress(spec.Topic),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	if got := conn.count(); got != 1 {
		t.Fatalf("dials: got %d want 1", got)
	}
	if !bps.SpecEqual(sub.Spec(), spec) {
		t.Fatal("second attachment: spec mismatch")
	}

	if err := pub.Publish(ctx, msg); err != nil {
		t.Fatal(err)
	}

	for i, a := range []bps.Attachment{pub, sub} {
		got := recvSink(t, a)
		if !got.WrappedChunk().Address().Equal(msg.WrappedChunk().Address()) {
			t.Fatalf("sink %d received the wrong message", i)
		}
	}
}

func TestBridgeRoleUpgrade(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, bridge, conn, underlay := newBridge(t)
	spec, _, msg := anchorCohort(t, topic(0x72), []byte("upgraded"))

	// A read-only attach first: it fixes the cohort but may not publish.
	sub, err := bridge.Attach(ctx, bps.AttachOptions{
		Peer:  underlay,
		Topic: swarm.NewAddress(spec.Topic),
		Spec:  spec,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	if err := sub.Publish(ctx, msg); err == nil {
		t.Fatal("read-only attachment published")
	}

	pub, err := bridge.Attach(ctx, bps.AttachOptions{
		Peer:  underlay,
		Topic: swarm.NewAddress(spec.Topic),
		Spec:  spec,
		Owner: spec.Admin,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	// The upgrade reuses the overlay learned by the first dial.
	if got := conn.count(); got != 1 {
		t.Fatalf("dials: got %d want 1", got)
	}

	if err := pub.Publish(ctx, msg); err != nil {
		t.Fatal(err)
	}

	for i, a := range []bps.Attachment{pub, sub} {
		got := recvSink(t, a)
		if !got.WrappedChunk().Address().Equal(msg.WrappedChunk().Address()) {
			t.Fatalf("sink %d received the wrong message", i)
		}
	}
}

func TestBridgeSpecMismatch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, bridge, _, underlay := newBridge(t)
	spec, _, _ := anchorCohort(t, topic(0x74), []byte("mismatched"))

	a, err := bridge.Attach(ctx, bps.AttachOptions{
		Peer:  underlay,
		Topic: swarm.NewAddress(spec.Topic),
		Spec:  spec,
		Owner: spec.Admin,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	// Same topic, different cohort: the live session verifies every inbound
	// message against its own spec, so a second client cannot be served on it
	// under different rules.
	other := &pb.CohortSpec{
		Topic:      spec.Topic,
		Binding:    spec.Binding,
		Publishers: spec.Publishers,
		Admin:      spec.Admin,
		Closed:     true,
	}
	if _, err := bridge.Attach(ctx, bps.AttachOptions{
		Peer:  underlay,
		Topic: swarm.NewAddress(spec.Topic),
		Spec:  other,
	}); !errors.Is(err, bps.ErrSpecMismatch) {
		t.Fatalf("attach with a conflicting spec: got %v want %v", err, bps.ErrSpecMismatch)
	}
}

func TestBridgeSlowSinkIsDroppedPast(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, bridge, _, underlay := newBridge(t)
	spec, signer, _ := anchorCohort(t, topic(0x75), []byte("slow"))

	att := func(owner []byte) bps.Attachment {
		t.Helper()
		a, err := bridge.Attach(ctx, bps.AttachOptions{
			Peer:  underlay,
			Topic: swarm.NewAddress(spec.Topic),
			Spec:  spec,
			Owner: owner,
		})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}

	pub := att(spec.Admin)
	defer pub.Close()
	fast := att(nil)
	slow := att(nil) // never drained
	defer slow.Close()

	// More messages than a sink's buffer holds, so the undrained one must
	// overflow while the drained one keeps up.
	const count = bps.OutboundQueueSize + 6

	// Drain the fast sink concurrently: the broker resets a peer whose own
	// outbound queue overflows, so the pipeline has to keep moving while the
	// publisher writes.
	got := make(chan int)
	go func() {
		n := 0
		for range fast.Messages() {
			n++
			if n == count {
				break
			}
		}
		got <- n
	}()

	for i := 0; i < count; i++ {
		msg := bpstesting.AnchorSOC(t, signer, topic(0x75), []byte{byte(i)})
		if err := pub.Publish(ctx, msg); err != nil {
			t.Fatal(err)
		}
	}

	select {
	case n := <-got:
		if n != count {
			t.Fatalf("fast sink: got %d messages want %d", n, count)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("fast sink did not keep up while another sink stalled")
	}
	if err := fast.Close(); err != nil {
		t.Fatal(err)
	}

	// The slow sink kept exactly a bufferful; the rest were dropped past it
	// rather than stalling the session. Closing it leaves the buffered
	// messages readable, so they can be counted.
	if err := slow.Close(); err != nil {
		t.Fatal(err)
	}
	n := 0
	for range slow.Messages() {
		n++
	}
	if n != bps.OutboundQueueSize {
		t.Fatalf("slow sink: got %d buffered messages want %d", n, bps.OutboundQueueSize)
	}
}

func TestBridgeSessionEndTearsDown(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	broker, bridge, _, underlay := newBridge(t)
	spec, _, _ := anchorCohort(t, topic(0x76), []byte("broker gone"))

	sub, err := bridge.Attach(ctx, bps.AttachOptions{
		Peer:  underlay,
		Topic: swarm.NewAddress(spec.Topic),
		Spec:  spec,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	// The broker goes away under the session: it resets every retained stream,
	// which ends the client session without anyone locally asking for it.
	if err := broker.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case _, ok := <-sub.Messages():
		if ok {
			t.Fatal("sink delivered a message after the broker went away")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("sink channel not closed after the session ended")
	}

	// The dead session must not linger in the status listing the API serves.
	eventually(t, func() bool {
		return len(bridge.Status()) == 0
	})
}

func TestBridgeLastCloseTearsDown(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	broker, bridge, _, underlay := newBridge(t)
	spec, _, _ := anchorCohort(t, topic(0x73), []byte("torn down"))

	pub, err := bridge.Attach(ctx, bps.AttachOptions{
		Peer:  underlay,
		Topic: swarm.NewAddress(spec.Topic),
		Spec:  spec,
		Owner: spec.Admin,
	})
	if err != nil {
		t.Fatal(err)
	}
	sub, err := bridge.Attach(ctx, bps.AttachOptions{
		Peer:  underlay,
		Topic: swarm.NewAddress(spec.Topic),
	})
	if err != nil {
		t.Fatal(err)
	}

	// One p2p stream carries both sinks.
	eventually(t, func() bool {
		st := broker.Status()
		return len(st) == 1 && st[0].Peers == 1
	})

	// The first detach leaves the session up for the remaining sink...
	if err := pub.Close(); err != nil {
		t.Fatal(err)
	}
	if st := broker.Status(); len(st) != 1 || st[0].Peers != 1 {
		t.Fatal("session torn down while a sink was still attached")
	}

	// ...and the last one takes it down.
	if err := sub.Close(); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool {
		st := broker.Status()
		return len(st) == 1 && st[0].Peers == 0
	})

	if _, ok := <-sub.Messages(); ok {
		t.Fatal("sink channel still open after teardown")
	}
	if len(bridge.Status()) != 0 {
		t.Fatalf("bridge status: got %d entries want 0", len(bridge.Status()))
	}
}
