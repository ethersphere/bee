// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/bps"
	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	bpstesting "github.com/ethersphere/bee/v2/pkg/bps/testing"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// anchorCohort builds a single-publisher ANCHOR cohort around one signer, and
// a helper that mints qualifying messages for it.
func anchorCohort(t *testing.T, id []byte, payload []byte) (*pb.CohortSpec, crypto.Signer, *soc.SOC) {
	t.Helper()

	signer, owner := bpstesting.NewSigner(t)
	s := bpstesting.AnchorSOC(t, signer, id, payload)
	anchor, err := s.Address()
	if err != nil {
		t.Fatal(err)
	}
	return &pb.CohortSpec{
		Topic:      anchor.Bytes(),
		Binding:    pb.TopicBinding_ANCHOR,
		Publishers: pb.PublisherRegime_EXPLICIT_SINGLE,
		Admin:      owner,
	}, signer, s
}

func recv(t *testing.T, ss *bps.Session) *soc.SOC {
	t.Helper()

	select {
	case s, ok := <-ss.Messages():
		if !ok {
			t.Fatal("session closed before delivering a message")
		}
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a broadcast")
		return nil
	}
}

func TestBroadcastReachesEveryPeer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	broker, _, brokerAddr := newBroker(t, bps.Options{})
	client := newClient(t, broker, brokerAddr)

	spec, _, msg := anchorCohort(t, topic(0x41), []byte("first message"))

	pub, err := client.Open(ctx, brokerAddr, spec, &pb.PublisherAuth{Owner: spec.Admin})
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	sub, err := client.Subscribe(ctx, brokerAddr, swarm.NewAddress(spec.Topic), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	if err := pub.Publish(ctx, msg); err != nil {
		t.Fatal(err)
	}

	// The subscriber receives it...
	got := recv(t, sub)
	if !got.WrappedChunk().Address().Equal(msg.WrappedChunk().Address()) {
		t.Fatal("subscriber received the wrong message")
	}
	// ...and so does the publisher, on its own stream.
	own := recv(t, pub)
	if !own.WrappedChunk().Address().Equal(msg.WrappedChunk().Address()) {
		t.Fatal("publisher did not receive its own message")
	}
}

func TestBroadcastDeduplicates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	broker, _, brokerAddr := newBroker(t, bps.Options{})
	client := newClient(t, broker, brokerAddr)

	spec, signer, first := anchorCohort(t, topic(0x51), []byte("first"))
	// A second message under the same id and owner — same anchor, different
	// payload, so it qualifies and is not a duplicate.
	second := bpstesting.AnchorSOC(t, signer, topic(0x51), []byte("second"))

	pub, err := client.Open(ctx, brokerAddr, spec, &pb.PublisherAuth{Owner: spec.Admin})
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	sub, err := client.Subscribe(ctx, brokerAddr, swarm.NewAddress(spec.Topic), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	if err := pub.Publish(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := pub.Publish(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := pub.Publish(ctx, second); err != nil {
		t.Fatal(err)
	}

	// The duplicate is dropped by the broker, so the subscriber sees
	// first then second, never first twice.
	if got := recv(t, sub); !got.WrappedChunk().Address().Equal(first.WrappedChunk().Address()) {
		t.Fatal("expected the first message")
	}
	if got := recv(t, sub); !got.WrappedChunk().Address().Equal(second.WrappedChunk().Address()) {
		t.Fatal("expected the second message; the duplicate was not dropped")
	}
}

func TestBroadcastDropsUnauthorizedMessage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	broker, _, brokerAddr := newBroker(t, bps.Options{})
	client := newClient(t, broker, brokerAddr)

	spec, _, msg := anchorCohort(t, topic(0x61), []byte("legitimate"))

	pub, err := client.Open(ctx, brokerAddr, spec, &pb.PublisherAuth{Owner: spec.Admin})
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	sub, err := client.Subscribe(ctx, brokerAddr, swarm.NewAddress(spec.Topic), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	// Under the explicit publisher regime the anchor binding no longer checks
	// the SOC address against the topic (SWIP-60: the id does no protocol
	// work there), so a message signed by an owner outside the cohort is now
	// refused by authorization instead: it qualifies but is not the admin.
	impostorSigner, _ := bpstesting.NewSigner(t)
	impostor := bpstesting.AnchorSOC(t, impostorSigner, spec.Topic, []byte("impostor"))
	if err := pub.Publish(ctx, impostor); err == nil {
		t.Fatal("expected the session to refuse a message from a non-listed owner")
	}

	// A legitimate message still gets through afterwards.
	if err := pub.Publish(ctx, msg); err != nil {
		t.Fatal(err)
	}
	if got := recv(t, sub); !got.WrappedChunk().Address().Equal(msg.WrappedChunk().Address()) {
		t.Fatal("expected the legitimate message")
	}
}

// TestPublisherAuthIsNotACredential pins that the owner a peer declares in its
// handshake buys it nothing. The broker admits it — the declared owner is
// unauthenticated, and admission is deliberately only an early refusal — but
// the authenticating check runs at publish time against the owner recovered
// from the message signature, so a peer that named someone else's address
// cannot get a message out to the cohort.
func TestPublisherAuthIsNotACredential(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	broker, streamer, brokerAddr := newBroker(t, bps.Options{})
	client := newClient(t, broker, brokerAddr)

	spec, _, msg := anchorCohort(t, topic(0x91), []byte("legitimate"))

	pub, err := client.Open(ctx, brokerAddr, spec, &pb.PublisherAuth{Owner: spec.Admin})
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	sub, err := client.Subscribe(ctx, brokerAddr, swarm.NewAddress(spec.Topic), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	// The impostor claims the admin's address, which it has no key for, and
	// is admitted as a publisher on that claim alone.
	_, w, _, ack := rawStream(t, streamer, brokerAddr, subscribeHello(spec.Topic, &pb.PublisherAuth{Owner: spec.Admin}))
	if ack.Status != pb.Status_OK {
		t.Fatalf("impostor admission: got %s want OK", ack.Status)
	}

	// It then publishes a message signed by its own key.
	impostorSigner, _ := bpstesting.NewSigner(t)
	impostorMsg := bpstesting.AnchorSOC(t, impostorSigner, topic(0x91), []byte("impostor"))
	m, err := bps.SocToProto(impostorMsg)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WriteMsgWithContext(ctx, &pb.Publish{Soc: m}); err != nil {
		t.Fatal(err)
	}

	// Give the broker a moment to have processed the impostor's frame before
	// the legitimate one is sent, so that "the subscriber's first message is
	// the legitimate one" really means the impostor's was dropped rather than
	// merely overtaken.
	time.Sleep(100 * time.Millisecond)

	if err := pub.Publish(ctx, msg); err != nil {
		t.Fatal(err)
	}

	got := recv(t, sub)
	if !got.WrappedChunk().Address().Equal(msg.WrappedChunk().Address()) {
		t.Fatal("the impostor's message reached a subscriber")
	}
}

// TestBroadcastDropsSlowPeer pins the design's slow-peer promise: a peer that
// stops draining fills its bounded outbound queue, is dropped from the cohort
// and has its stream reset, and the cohort goes on serving everyone else.
// Without it, one stalled reader would back the broker's fan-out up behind it.
//
// The drop is observed through the cohort's capacity — a slot the dropped peer
// held becomes free — rather than by watching its stream end. streamtest's
// in-memory pipe holds its record lock across a blocked write, so once the
// broker's write to a peer that never reads has jammed, nothing can close or
// drain that pipe from either side. That is a harness artifact, not the
// behaviour under test: a real libp2p Reset does not wait on the writer.
func TestBroadcastDropsSlowPeer(t *testing.T) {
	t.Parallel()

	// Enough to overrun the slow peer's outbound queue: the broker's writes to
	// it park once its stream stops being drained, and everything after that
	// piles up in the queue until it overflows.
	const storm = 3 * bps.OutboundQueueSize

	ctx := context.Background()
	// Exactly three slots: the publisher, the healthy subscriber and the slow
	// peer. A fourth handshake is refused while the slow peer holds its slot,
	// and admitted once it has been dropped.
	broker, streamer, brokerAddr := newBroker(t, bps.Options{Capacity: 3})
	client := newClient(t, broker, brokerAddr)

	spec, signer, _ := anchorCohort(t, topic(0xa0), []byte("slow peer"))

	pub, err := client.Open(ctx, brokerAddr, spec, &pb.PublisherAuth{Owner: spec.Admin})
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	healthy, err := client.Subscribe(ctx, brokerAddr, swarm.NewAddress(spec.Topic), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer healthy.Close()

	// The slow peer completes the handshake and then never reads again. Its
	// stream is deliberately not reset on cleanup: see the note above on the
	// jammed pipe.
	slowStream, err := streamer.NewStream(ctx, brokerAddr, nil, bps.ProtocolName, bps.ProtocolVersion, bps.StreamName)
	if err != nil {
		t.Fatal(err)
	}
	slowWriter, slowReader := protobuf.NewWriterAndReader(slowStream)
	if err := slowWriter.WriteMsgWithContext(ctx, subscribeHello(spec.Topic, nil)); err != nil {
		t.Fatal(err)
	}
	var slowAck pb.Ack
	if err := slowReader.ReadMsgWithContext(ctx, &slowAck); err != nil {
		t.Fatal(err)
	}
	if slowAck.Status != pb.Status_OK {
		t.Fatalf("slow peer admission: got %s want OK", slowAck.Status)
	}

	// With every slot taken, a further peer is refused.
	if ack := handshake(t, streamer, brokerAddr, subscribeHello(spec.Topic, nil)); ack.Status != pb.Status_FULL {
		t.Fatalf("cohort should be full: got %s want FULL", ack.Status)
	}

	// The publisher's own copy of every broadcast is drained continuously so
	// that it, unlike the slow peer, never fills its own queue.
	go func() {
		for range pub.Messages() {
		}
	}()

	// Publishing is paced against the healthy subscriber, one message at a
	// time. streamtest's in-memory pipe can only carry a bounded number of
	// unconsumed writes, so a tight publish loop would stall the test harness
	// itself long before it stalled the broker. The slow peer is the only
	// party here that is deliberately not kept up with.
	publish := func(payload string) *soc.SOC {
		t.Helper()

		m := bpstesting.AnchorSOC(t, signer, topic(0xa0), []byte(payload))
		if err := pub.Publish(ctx, m); err != nil {
			t.Fatal(err)
		}
		return m
	}

	for i := range storm {
		publish(fmt.Sprintf("storm %d", i))
		if got := recv(t, healthy); got == nil {
			t.Fatal("healthy subscriber stopped receiving mid-storm")
		}
	}

	// The slow peer's slot is free again: it was dropped from the cohort, not
	// merely left behind on its own stream.
	if ack := handshake(t, streamer, brokerAddr, subscribeHello(spec.Topic, nil)); ack.Status != pb.Status_OK {
		t.Fatalf("slow peer was not dropped: got %s want OK", ack.Status)
	}

	// The cohort keeps serving: a message published after the drop still
	// reaches the healthy subscriber.
	sentinel := publish("after the drop")
	if got := recv(t, healthy); !got.WrappedChunk().Address().Equal(sentinel.WrappedChunk().Address()) {
		t.Fatal("the cohort stopped serving after dropping the slow peer")
	}
}

// TestSessionConcurrentPublish pins that Publish is safe for concurrent use,
// as its exported contract promises. The session's writer wraps mutable
// framing state; unserialised, two goroutines interleave bytes on the wire and
// desynchronise the broker's framing for good. Run under -race this catches
// the data race directly; without it, the corrupted framing shows up as
// messages that never arrive.
func TestSessionConcurrentPublish(t *testing.T) {
	t.Parallel()

	// Rounds of concurrent writers, rather than one long tight loop: the
	// writers within a round really do race each other for the session's
	// writer, while draining between rounds keeps streamtest's in-memory pipe
	// from stalling on unconsumed writes.
	const (
		writers = 8
		rounds  = 5
	)

	ctx := context.Background()
	broker, _, brokerAddr := newBroker(t, bps.Options{})
	client := newClient(t, broker, brokerAddr)

	spec, signer, _ := anchorCohort(t, topic(0xb0), []byte("concurrent"))

	pub, err := client.Open(ctx, brokerAddr, spec, &pb.PublisherAuth{Owner: spec.Admin})
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()

	sub, err := client.Subscribe(ctx, brokerAddr, swarm.NewAddress(spec.Topic), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	// The publisher's own copy of every broadcast is drained continuously.
	go func() {
		for range pub.Messages() {
		}
	}()

	seen := make(map[string]struct{}, writers*rounds)
	for round := range rounds {
		start := make(chan struct{})
		var wg sync.WaitGroup
		for w := range writers {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()

				m := bpstesting.AnchorSOC(t, signer, topic(0xb0), []byte(fmt.Sprintf("round %d writer %d", round, w)))
				<-start
				if err := pub.Publish(ctx, m); err != nil {
					t.Errorf("publish: %v", err)
				}
			}(w)
		}
		close(start)
		wg.Wait()

		// Every distinct message arrives exactly once: nothing was lost to a
		// mangled frame, and nothing was duplicated.
		for range writers {
			m := recv(t, sub)
			addr := m.WrappedChunk().Address().String()
			if _, ok := seen[addr]; ok {
				t.Fatalf("message %s delivered twice", addr)
			}
			seen[addr] = struct{}{}
		}
	}
	if len(seen) != writers*rounds {
		t.Fatalf("delivered %d distinct messages, want %d", len(seen), writers*rounds)
	}
}

// TestCloseWithLivePublisherStream ensures Close does not exceed its 5-second
// budget when a publisher stream is still attached: the broker's serve call
// for that stream has a reader goroutine potentially blocked in ReadMsg on
// the very same stream Close's teardown must also touch. If Close (or serve)
// ever went back to resetting/closing that stream in a way that waits on the
// stream itself while the reader is also blocked on it, this would hang for
// the full 5 seconds instead of returning promptly.
func TestCloseWithLivePublisherStream(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	broker, _, brokerAddr := newBroker(t, bps.Options{})
	client := newClient(t, broker, brokerAddr)

	spec, _, _ := anchorCohort(t, topic(0x81), []byte("still attached"))

	pub, err := client.Open(ctx, brokerAddr, spec, &pb.PublisherAuth{Owner: spec.Admin})
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()
	// Deliberately not closed before broker.Close() below: the broker's
	// serve and readPublished goroutines for this stream are still live.

	done := make(chan error, 1)
	go func() {
		done <- broker.Close()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("got %v want nil", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("Close did not return within 4 seconds of its 5-second budget")
	}
}

func TestCloseTearsDownSessions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	broker, _, brokerAddr := newBroker(t, bps.Options{})
	client := newClient(t, broker, brokerAddr)

	spec, _, _ := anchorCohort(t, topic(0x71), []byte("teardown"))

	sess, err := client.Open(ctx, brokerAddr, spec, &pb.PublisherAuth{Owner: spec.Admin})
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case _, ok := <-sess.Messages():
		if ok {
			t.Fatal("expected the message channel to be closed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the message channel to close")
	}
}
