// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sim

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/pullsync/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type msgCollector struct {
	mu   sync.Mutex
	msgs []MsgEvent
}

func (c *msgCollector) add(m MsgEvent) {
	c.mu.Lock()
	c.msgs = append(c.msgs, m)
	c.mu.Unlock()
}

func (c *msgCollector) snapshot() []MsgEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]MsgEvent, len(c.msgs))
	copy(out, c.msgs)
	return out
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}

func newPair(t *testing.T, handler p2p.HandlerFunc, hooks TransportHooks) (*Transport, swarm.Address) {
	t.Helper()
	rng := rand.New(rand.NewSource(99))
	client := randAddr(rng)
	server := randAddr(rng)

	tr := NewTransport(client, 0, hooks)
	tr.SetHandler(server, p2p.ProtocolSpec{
		Name:    "pullsync",
		Version: "1.4.0",
		StreamSpecs: []p2p.StreamSpec{
			{Name: streamNamePullsync, Handler: handler},
			{Name: streamNameCursors, Handler: handler},
		},
	})
	t.Cleanup(func() { _ = tr.Close() })
	return tr, server
}

// TestTransport_DeliveryBurst is the regression guard: streamtest deadlocks
// past 16 buffered writes, our transport must not.
func TestTransport_DeliveryBurst(t *testing.T) {
	t.Parallel()

	const n = 200
	handler := func(_ context.Context, _ p2p.Peer, stream p2p.Stream) error {
		w := protobuf.NewWriter(stream)
		for i := 0; i < n; i++ {
			if err := w.WriteMsg(&pb.Delivery{Address: make([]byte, swarm.HashSize)}); err != nil {
				return err
			}
		}
		return stream.FullClose()
	}

	tr, server := newPair(t, handler, TransportHooks{})

	stream, err := tr.NewStream(context.Background(), server, nil, "pullsync", "1.4.0", streamNamePullsync)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.FullClose()

	done := make(chan int, 1)
	go func() {
		r := protobuf.NewReader(stream)
		count := 0
		for {
			var d pb.Delivery
			if err := r.ReadMsg(&d); err != nil {
				break
			}
			count++
		}
		done <- count
	}()

	select {
	case got := <-done:
		if got != n {
			t.Fatalf("expected %d deliveries, got %d", n, got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock: burst delivery did not complete")
	}
}

// TestTransport_WireTapPullsync asserts the decoded frame sequence and
// direction attribution for a full sync round.
func TestTransport_WireTapPullsync(t *testing.T) {
	t.Parallel()

	col := &msgCollector{}
	handler := func(_ context.Context, _ p2p.Peer, stream p2p.Stream) error {
		w, r := protobuf.NewWriterAndReader(stream)
		var g pb.Get
		if err := r.ReadMsg(&g); err != nil {
			return err
		}
		offer := &pb.Offer{Topmost: 5, Chunks: []*pb.Chunk{
			{Address: make([]byte, swarm.HashSize)},
			{Address: make([]byte, swarm.HashSize)},
		}}
		if err := w.WriteMsg(offer); err != nil {
			return err
		}
		var want pb.Want
		if err := r.ReadMsg(&want); err != nil {
			return err
		}
		for i := 0; i < 2; i++ {
			if err := w.WriteMsg(&pb.Delivery{Address: make([]byte, swarm.HashSize)}); err != nil {
				return err
			}
		}
		return stream.FullClose()
	}

	tr, server := newPair(t, handler, TransportHooks{OnMsg: col.add})

	stream, err := tr.NewStream(context.Background(), server, nil, "pullsync", "1.4.0", streamNamePullsync)
	if err != nil {
		t.Fatal(err)
	}
	w, r := protobuf.NewWriterAndReader(stream)

	if err := w.WriteMsg(&pb.Get{Bin: 3, Start: 1}); err != nil {
		t.Fatal(err)
	}
	var offer pb.Offer
	if err := r.ReadMsg(&offer); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteMsg(&pb.Want{BitVector: []byte{0x03}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		var d pb.Delivery
		if err := r.ReadMsg(&d); err != nil {
			t.Fatal(err)
		}
	}
	_ = stream.FullClose()

	waitFor(t, 3*time.Second, func() bool { return len(col.snapshot()) >= 5 })

	msgs := col.snapshot()
	var c2s, s2c []MsgEvent
	for _, m := range msgs {
		if m.Dir == dirC2S {
			c2s = append(c2s, m)
		} else {
			s2c = append(s2c, m)
		}
	}

	if len(c2s) != 2 || c2s[0].Type != MsgGet || c2s[1].Type != MsgWant {
		t.Fatalf("unexpected c2s sequence: %+v", c2s)
	}
	if c2s[0].Bin != 3 {
		t.Fatalf("expected Get bin 3, got %d", c2s[0].Bin)
	}
	if c2s[1].N != 2 {
		t.Fatalf("expected Want popcount 2, got %d", c2s[1].N)
	}
	if len(s2c) != 3 || s2c[0].Type != MsgOffer || s2c[1].Type != MsgDelivery || s2c[2].Type != MsgDelivery {
		t.Fatalf("unexpected s2c sequence: %+v", s2c)
	}
	if s2c[0].N != 2 {
		t.Fatalf("expected Offer count 2, got %d", s2c[0].N)
	}
}

// TestTransport_WireTapCursors checks Syn/Ack classification.
func TestTransport_WireTapCursors(t *testing.T) {
	t.Parallel()

	col := &msgCollector{}
	handler := func(_ context.Context, _ p2p.Peer, stream p2p.Stream) error {
		w, r := protobuf.NewWriterAndReader(stream)
		var syn pb.Syn
		if err := r.ReadMsg(&syn); err != nil {
			return err
		}
		if err := w.WriteMsg(&pb.Ack{Cursors: make([]uint64, 8), Epoch: 1}); err != nil {
			return err
		}
		return stream.FullClose()
	}

	tr, server := newPair(t, handler, TransportHooks{OnMsg: col.add})
	stream, err := tr.NewStream(context.Background(), server, nil, "pullsync", "1.4.0", streamNameCursors)
	if err != nil {
		t.Fatal(err)
	}
	w, r := protobuf.NewWriterAndReader(stream)
	if err := w.WriteMsg(&pb.Syn{}); err != nil {
		t.Fatal(err)
	}
	var ack pb.Ack
	if err := r.ReadMsg(&ack); err != nil {
		t.Fatal(err)
	}
	_ = stream.FullClose()

	waitFor(t, 3*time.Second, func() bool { return len(col.snapshot()) >= 2 })
	msgs := col.snapshot()
	var haveSyn, haveAck bool
	for _, m := range msgs {
		if m.Type == MsgSyn && m.Dir == dirC2S {
			haveSyn = true
		}
		if m.Type == MsgAck && m.Dir == dirS2C {
			haveAck = true
		}
	}
	if !haveSyn || !haveAck {
		t.Fatalf("expected Syn and Ack, got %+v", msgs)
	}
}

// TestTransport_ResetCancelsHandler verifies a client Reset cancels the parked
// server handler context.
func TestTransport_ResetCancelsHandler(t *testing.T) {
	t.Parallel()

	cancelled := make(chan struct{})
	handler := func(ctx context.Context, _ p2p.Peer, stream p2p.Stream) error {
		<-ctx.Done()
		close(cancelled)
		return stream.Reset()
	}

	tr, server := newPair(t, handler, TransportHooks{})
	stream, err := tr.NewStream(context.Background(), server, nil, "pullsync", "1.4.0", streamNamePullsync)
	if err != nil {
		t.Fatal(err)
	}

	if err := stream.Reset(); err != nil {
		t.Fatal(err)
	}

	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("handler context was not cancelled on reset")
	}
}

// TestTransport_CloseUnblocksParkedHandler verifies transport Close cancels a
// handler blocked reading.
func TestTransport_CloseUnblocksParkedHandler(t *testing.T) {
	t.Parallel()

	returned := make(chan struct{})
	handler := func(ctx context.Context, _ p2p.Peer, stream p2p.Stream) error {
		r := protobuf.NewReader(stream)
		var g pb.Get
		// Blocks until the stream is torn down (returns error), then returns.
		_ = r.ReadMsgWithContext(ctx, &g)
		close(returned)
		return nil
	}

	tr, server := newPair(t, handler, TransportHooks{})
	_, err := tr.NewStream(context.Background(), server, nil, "pullsync", "1.4.0", streamNamePullsync)
	if err != nil {
		t.Fatal(err)
	}

	_ = tr.Close()

	select {
	case <-returned:
	case <-time.After(3 * time.Second):
		t.Fatal("parked handler did not return after transport Close")
	}
}

// TestTransport_NoMsgAfterStreamClose guards the wire-tap ordering contract:
// the taps decode asynchronously, so without draining them on close the
// consumer sees Offer/Delivery events for a stream it was already told had
// ended. The event bus deletes its per-stream aggregate on close, so a late
// Delivery would resurrect it with no Offer recorded and render as "DLV n/0".
func TestTransport_NoMsgAfterStreamClose(t *testing.T) {
	t.Parallel()

	const n = 64
	handler := func(_ context.Context, _ p2p.Peer, stream p2p.Stream) error {
		w := protobuf.NewWriter(stream)
		offered := make([]*pb.Chunk, n)
		for i := range offered {
			offered[i] = &pb.Chunk{Address: make([]byte, swarm.HashSize)}
		}
		if err := w.WriteMsg(&pb.Offer{Topmost: 1, Chunks: offered}); err != nil {
			return err
		}
		for i := 0; i < n; i++ {
			if err := w.WriteMsg(&pb.Delivery{Address: make([]byte, swarm.HashSize)}); err != nil {
				return err
			}
		}
		return stream.FullClose()
	}

	var (
		mu         sync.Mutex
		closed     = map[uint64]bool{}
		afterClose int
	)
	hooks := TransportHooks{
		OnMsg: func(m MsgEvent) {
			mu.Lock()
			defer mu.Unlock()
			if closed[m.StreamID] {
				afterClose++
			}
		},
		OnStream: func(s StreamEvent) {
			mu.Lock()
			defer mu.Unlock()
			if s.Phase == StreamClose {
				closed[s.StreamID] = true
			}
		},
	}

	tr, server := newPair(t, handler, hooks)

	stream, err := tr.NewStream(context.Background(), server, nil, "pullsync", "1.4.0", streamNamePullsync)
	if err != nil {
		t.Fatal(err)
	}
	r := protobuf.NewReader(stream)
	var offer pb.Offer
	if err := r.ReadMsg(&offer); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		var d pb.Delivery
		if err := r.ReadMsg(&d); err != nil {
			t.Fatal(err)
		}
	}
	_ = stream.FullClose()
	_ = tr.Close()

	mu.Lock()
	defer mu.Unlock()
	if afterClose != 0 {
		t.Fatalf("%d wire messages emitted after their stream close event", afterClose)
	}
}
