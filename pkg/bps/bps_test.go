// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/bps"
	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const waitFor = 5 * time.Second

type blocklister struct {
	mu    sync.Mutex
	peers map[string]string
}

func (b *blocklister) NetworkStatus() p2p.NetworkStatus { return p2p.NetworkStatusAvailable }

func (b *blocklister) Blocklist(overlay swarm.Address, _ time.Duration, reason string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.peers[overlay.ByteString()] = reason
	return nil
}

func (b *blocklister) blocklisted(overlay swarm.Address) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.peers[overlay.ByteString()]
	return ok
}

type env struct {
	t          *testing.T
	broker     *bps.Service
	brokerAddr swarm.Address
	bl         *blocklister
	signer     crypto.Signer
	principal  []byte
	topic      []byte
}

func newEnv(t *testing.T, o bps.Options) *env {
	t.Helper()
	bl := &blocklister{peers: make(map[string]string)}
	broker := bps.New(nil, bl, true, log.Noop, o)
	t.Cleanup(func() { _ = broker.Close() })
	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(key)
	admin, err := signer.EthereumAddress()
	if err != nil {
		t.Fatal(err)
	}
	topic := make([]byte, swarm.HashSize)
	copy(topic, "bps-test-topic")
	return &env{t: t, broker: broker, brokerAddr: swarm.RandAddress(t), bl: bl, signer: signer, principal: admin.Bytes(), topic: topic}
}

// client returns a node whose streams reach the broker from overlay.
func (e *env) client(overlay swarm.Address) (*bps.Service, *streamtest.Recorder) {
	rec := streamtest.New(streamtest.WithProtocols(e.broker.Protocol()), streamtest.WithBaseAddr(overlay))
	return bps.New(rec, nil, false, log.Noop, bps.Options{}), rec
}

func (e *env) join(c *bps.Service, addr []byte) bps.Session {
	e.t.Helper()
	s, err := c.Join(context.Background(), bps.JoinRequest{Broker: e.brokerAddr, Spec: bps.CohortSpec{Topic: e.topic, Principal: e.principal}, Identity: addr})
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _ = s.Close() })
	return s
}

func (e *env) subscribe() bps.Session {
	e.t.Helper()
	c, _ := e.client(swarm.RandAddress(e.t))
	addr := make([]byte, 20)
	copy(addr, swarm.RandAddress(e.t).Bytes())
	return e.join(c, addr)
}

func (e *env) publisher() (bps.Session, swarm.Address) {
	e.t.Helper()
	overlay := swarm.RandAddress(e.t)
	c, _ := e.client(overlay)
	return e.join(c, e.principal), overlay
}

func (e *env) chunk(signer crypto.Signer, kind bps.Kind, challenge []byte, index uint64, payload []byte) []byte {
	e.t.Helper()
	id, err := bps.ID(kind, e.topic, challenge, index)
	if err != nil {
		e.t.Fatal(err)
	}
	ch, err := cac.New(payload)
	if err != nil {
		e.t.Fatal(err)
	}
	s, err := soc.New(id, ch).Sign(signer)
	if err != nil {
		e.t.Fatal(err)
	}
	return s.Data()
}

func (e *env) publish(s bps.Session, kind bps.Kind, index uint64, payload []byte) []byte {
	e.t.Helper()
	c := e.chunk(e.signer, kind, s.Challenge(), index, payload)
	if err := s.Publish(context.Background(), kind, index, c); err != nil {
		e.t.Fatal(err)
	}
	return c
}

func (e *env) counters() bps.Counters {
	e.t.Helper()
	c, ok := e.broker.Counters(bps.CohortSpec{Topic: e.topic, Principal: e.principal})
	if !ok {
		e.t.Fatal("no live cohort")
	}
	return c
}

func expectMessage(t *testing.T, s bps.Session, index uint64, chunk []byte) {
	t.Helper()
	select {
	case m := <-s.Messages():
		if m.Index != index || !bytes.Equal(m.SOC, chunk) {
			t.Fatalf("got index %d %x, want %d %x", m.Index, m.SOC, index, chunk)
		}
	case <-time.After(waitFor):
		t.Fatalf("timed out waiting for index %d", index)
	}
}

func expectNoMessage(t *testing.T, s bps.Session) {
	t.Helper()
	select {
	case m, ok := <-s.Messages():
		if ok {
			t.Fatalf("unexpected message at index %d", m.Index)
		}
	case <-time.After(100 * time.Millisecond):
	}
}

func expectDone(t *testing.T, s bps.Session) {
	t.Helper()
	select {
	case <-s.Done():
	case <-time.After(waitFor):
		t.Fatal("stream not ended")
	}
}

func eventually(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitFor)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestBroadcast(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	sub := e.subscribe()
	pub, _ := e.publisher()

	if len(pub.Challenge()) != bps.ChallengeSize || bytes.Equal(pub.Challenge(), sub.Challenge()) {
		t.Fatal("want a distinct 32-byte challenge per stream")
	}

	// AUTH authenticates and is never delivered
	e.publish(pub, bps.KindAuth, 0, nil)
	c0 := e.publish(pub, bps.KindData, 0, []byte("hello cohort"))
	expectMessage(t, sub, 0, c0)
	expectNoMessage(t, pub)
}

func TestCursor(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	sub := e.subscribe()
	pub, _ := e.publisher()

	c5 := e.publish(pub, bps.KindData, 5, []byte("five")) // gaps are allowed
	expectMessage(t, sub, 5, c5)
	e.publish(pub, bps.KindData, 3, []byte("three")) // retransmit
	e.publish(pub, bps.KindData, 5, []byte("five"))  // retransmit
	c6 := e.publish(pub, bps.KindData, 6, []byte("six"))
	expectMessage(t, sub, 6, c6)
	if got := e.counters().Retransmit; got != 2 {
		t.Fatalf("retransmit %d, want 2", got)
	}
}

func TestPublisherReconnects(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	sub := e.subscribe()

	pub1, _ := e.publisher()
	for i := uint64(0); i < 3; i++ {
		expectMessage(t, sub, i, e.publish(pub1, bps.KindData, i, []byte{byte(i)}))
	}
	_ = pub1.Close()
	expectDone(t, pub1)

	// the cohort and its cursor persist; a valid frame below the cursor
	// still authenticates the new stream and is counted as a retransmit
	pub2, _ := e.publisher()
	if bytes.Equal(pub1.Challenge(), pub2.Challenge()) {
		t.Fatal("challenge reused")
	}
	e.publish(pub2, bps.KindData, 1, []byte{1})
	c3 := e.publish(pub2, bps.KindData, 3, []byte{3})
	expectMessage(t, sub, 3, c3)
	if got := e.counters().Retransmit; got != 1 {
		t.Fatalf("retransmit %d, want 1", got)
	}
}

func TestTwoPublisherStreams(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	sub := e.subscribe()
	pub1, _ := e.publisher()
	pub2, _ := e.publisher()

	c0 := e.publish(pub1, bps.KindData, 0, []byte("a"))
	expectMessage(t, sub, 0, c0)
	e.publish(pub2, bps.KindData, 0, []byte("a")) // signed for its own stream, delivered once
	c1 := e.publish(pub2, bps.KindData, 1, []byte("b"))
	expectMessage(t, sub, 1, c1)
	expectNoMessage(t, pub1)
}

func TestViolations(t *testing.T) {
	t.Parallel()

	other, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	otherSigner := crypto.NewDefaultSigner(other)

	t.Run("publish on a subscriber session", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, bps.Options{})
		sub := e.subscribe()
		c := e.chunk(e.signer, bps.KindData, sub.Challenge(), 0, []byte("x"))
		if err := sub.Publish(context.Background(), bps.KindData, 0, c); !errors.Is(err, bps.ErrNotPublisher) {
			t.Fatalf("got %v, want %v", err, bps.ErrNotPublisher)
		}
	})

	t.Run("publication from a subscriber stream", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, bps.Options{})
		overlay := swarm.RandAddress(t)
		keep := e.subscribe()
		w, r, ack := e.rawFrom(overlay, &pb.Join{Cohort: e.spec(), Identity: swarm.RandAddress(t).Bytes()[:20]})
		if ack.Status != pb.Status_OK {
			t.Fatalf("status %s", ack.Status)
		}
		c := e.chunk(e.signer, bps.KindData, ack.Challenge, 0, []byte("x"))
		if err := w.WriteMsg(&pb.Broadcast{Kind: pb.Kind_DATA, Challenge: ack.Challenge, Soc: c}); err != nil {
			t.Fatal(err)
		}
		if err := r.ReadMsg(&pb.Broadcast{}); err == nil {
			t.Fatal("stream not ended")
		}
		eventually(t, func() bool { return e.bl.blocklisted(overlay) })
		if got := e.counters().WrongStream; got != 1 {
			t.Fatalf("wrong_stream %d, want 1", got)
		}
		expectNoMessage(t, keep)
	})

	for _, tc := range []struct {
		name  string
		chunk func(e *env, challenge []byte) (bps.Kind, uint64, []byte)
	}{
		{"wrong signer", func(e *env, ch []byte) (bps.Kind, uint64, []byte) {
			return bps.KindData, 0, e.chunk(otherSigner, bps.KindData, ch, 0, []byte("x"))
		}},
		{"signed for another challenge", func(e *env, _ []byte) (bps.Kind, uint64, []byte) {
			return bps.KindData, 0, e.chunk(e.signer, bps.KindData, make([]byte, 32), 0, []byte("x"))
		}},
		{"index not the signed one", func(e *env, ch []byte) (bps.Kind, uint64, []byte) {
			return bps.KindData, 1, e.chunk(e.signer, bps.KindData, ch, 0, []byte("x"))
		}},
		{"auth relabelled as data", func(e *env, ch []byte) (bps.Kind, uint64, []byte) {
			return bps.KindData, 0, e.chunk(e.signer, bps.KindAuth, ch, 0, nil)
		}},
		{"auth with payload", func(e *env, ch []byte) (bps.Kind, uint64, []byte) {
			return bps.KindAuth, 0, e.chunk(e.signer, bps.KindAuth, ch, 0, []byte("x"))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t, bps.Options{})
			sub := e.subscribe()
			pub, overlay := e.publisher()
			kind, index, chunk := tc.chunk(e, pub.Challenge())
			if err := pub.Publish(context.Background(), kind, index, chunk); err != nil {
				t.Fatal(err)
			}
			expectDone(t, pub)
			eventually(t, func() bool { return e.bl.blocklisted(overlay) })
			if got := e.counters().InvalidSOC; got != 1 {
				t.Fatalf("invalid_soc %d, want 1", got)
			}
			expectNoMessage(t, sub)
		})
	}
}

// raw opens a stream to the broker and joins with join, returning the Ack.
func (e *env) raw(join *pb.Join) (protobuf.Writer, protobuf.Reader, *pb.Ack) {
	e.t.Helper()
	return e.rawFrom(swarm.RandAddress(e.t), join)
}

// rawFrom is raw from the given overlay.
func (e *env) rawFrom(overlay swarm.Address, join *pb.Join) (protobuf.Writer, protobuf.Reader, *pb.Ack) {
	e.t.Helper()
	rec := streamtest.New(streamtest.WithProtocols(e.broker.Protocol()), streamtest.WithBaseAddr(overlay))
	stream, err := rec.NewStream(context.Background(), e.brokerAddr, nil, bps.ProtocolName, bps.ProtocolVersion, bps.StreamName)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _ = stream.Reset() })
	w, r := protobuf.NewWriterAndReader(stream)
	if err := w.WriteMsg(join); err != nil {
		e.t.Fatal(err)
	}
	var ack pb.Ack
	if err := r.ReadMsg(&ack); err != nil {
		e.t.Fatal(err)
	}
	return w, r, &ack
}

func (e *env) spec() *pb.CohortSpec {
	return &pb.CohortSpec{Topic: e.topic, Binding: pb.TopicBinding_FEED_TOPIC, Principal: e.principal}
}

func TestDroppedFrames(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	sub := e.subscribe()
	w, _, ack := e.raw(&pb.Join{Cohort: e.spec(), Identity: e.principal})
	if ack.Status != pb.Status_OK {
		t.Fatalf("status %s", ack.Status)
	}
	write := func(f *pb.Broadcast) {
		t.Helper()
		if err := w.WriteMsg(f); err != nil {
			t.Fatal(err)
		}
	}

	// another kind and another challenge are dropped, not punished
	write(&pb.Broadcast{Kind: pb.Kind(7), Challenge: ack.Challenge, Soc: []byte("roster")})
	other := make([]byte, 32)
	write(&pb.Broadcast{Kind: pb.Kind_DATA, Challenge: other, Soc: e.chunk(e.signer, bps.KindData, other, 0, []byte("old"))})
	c := e.chunk(e.signer, bps.KindData, ack.Challenge, 0, []byte("ok"))
	write(&pb.Broadcast{Kind: pb.Kind_DATA, Challenge: ack.Challenge, Index: 0, Soc: c})
	expectMessage(t, sub, 0, c)

	got := e.counters()
	if got.UnknownKind != 1 || got.WrongChallenge != 1 || got.InvalidSOC != 0 {
		t.Fatalf("counters %+v", got)
	}
}

// TestInvalidDelivery checks that a subscriber ends the session when the
// broker forwards a chunk that does not verify against the cohort spec.
func TestInvalidDelivery(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	other, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	challenge := make([]byte, bps.ChallengeSize)
	bad := e.chunk(crypto.NewDefaultSigner(other), bps.KindData, challenge, 0, []byte("x"))
	broker := p2p.ProtocolSpec{
		Name:    bps.ProtocolName,
		Version: bps.ProtocolVersion,
		StreamSpecs: []p2p.StreamSpec{{
			Name: bps.StreamName,
			Handler: func(ctx context.Context, _ p2p.Peer, stream p2p.Stream) error {
				w, r := protobuf.NewWriterAndReader(stream)
				if err := r.ReadMsgWithContext(ctx, &pb.Join{}); err != nil {
					return err
				}
				if err := w.WriteMsgWithContext(ctx, &pb.Ack{Status: pb.Status_OK, Challenge: challenge}); err != nil {
					return err
				}
				if err := w.WriteMsgWithContext(ctx, &pb.Broadcast{Kind: pb.Kind_DATA, Challenge: challenge, Soc: bad}); err != nil {
					return err
				}
				// hold the stream until the subscriber ends it
				_ = r.ReadMsgWithContext(ctx, &pb.Broadcast{})
				return nil
			},
		}},
	}
	rec := streamtest.New(streamtest.WithProtocols(broker), streamtest.WithBaseAddr(swarm.RandAddress(t)))
	c := bps.New(rec, nil, false, log.Noop, bps.Options{})
	sub := e.join(c, swarm.RandAddress(t).Bytes()[:20])
	expectDone(t, sub)
	if err := sub.Err(); !errors.Is(err, bps.ErrInvalidDelivery) {
		t.Fatalf("got %v, want %v", err, bps.ErrInvalidDelivery)
	}
}

func TestJoinRejected(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	for _, tc := range []struct {
		name string
		join *pb.Join
	}{
		{"no cohort", &pb.Join{Identity: e.principal}},
		{"unspecified binding", &pb.Join{Cohort: &pb.CohortSpec{Topic: e.topic, Principal: e.principal}, Identity: e.principal}},
		{"short topic", &pb.Join{Cohort: &pb.CohortSpec{Topic: e.topic[:31], Binding: pb.TopicBinding_FEED_TOPIC, Principal: e.principal}, Identity: e.principal}},
		{"no admin", &pb.Join{Cohort: &pb.CohortSpec{Topic: e.topic, Binding: pb.TopicBinding_FEED_TOPIC}, Identity: e.principal}},
		{"short addr", &pb.Join{Cohort: e.spec(), Identity: e.principal[:19]}},
	} {
		_, _, ack := e.raw(tc.join)
		if ack.Status != pb.Status_REJECTED || len(ack.Challenge) != 0 {
			t.Fatalf("%s: got %s", tc.name, ack.Status)
		}
	}
}

func TestBounds(t *testing.T) {
	t.Parallel()

	t.Run("subscribers per cohort, principal admitted outside", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, bps.Options{MaxSubscribers: 1})
		sub := e.subscribe()
		if _, _, ack := e.raw(&pb.Join{Cohort: e.spec(), Identity: make([]byte, 20)}); ack.Status != pb.Status_FULL {
			t.Fatalf("got %s, want FULL", ack.Status)
		}
		pub, _ := e.publisher()
		expectMessage(t, sub, 0, e.publish(pub, bps.KindData, 0, []byte("x")))
	})

	t.Run("cohorts per broker", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, bps.Options{MaxCohorts: 1})
		e.subscribe()
		spec := e.spec()
		spec.Topic = make([]byte, 32)
		if _, _, ack := e.raw(&pb.Join{Cohort: spec, Identity: e.principal}); ack.Status != pb.Status_FULL {
			t.Fatalf("got %s, want FULL", ack.Status)
		}
	})

	t.Run("streams and cohorts per peer", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, bps.Options{MaxStreamsPerPeerCohort: 1, MaxCohortsPerPeer: 1})
		c, _ := e.client(swarm.RandAddress(t))
		e.join(c, e.principal)
		if _, err := c.Join(context.Background(), bps.JoinRequest{Broker: e.brokerAddr, Spec: bps.CohortSpec{Topic: e.topic, Principal: e.principal}, Identity: e.principal}); err == nil {
			t.Fatal("want a second stream on the cohort refused")
		}
		other := make([]byte, 32)
		if _, err := c.Join(context.Background(), bps.JoinRequest{Broker: e.brokerAddr, Spec: bps.CohortSpec{Topic: other, Principal: e.principal}, Identity: e.principal}); err == nil {
			t.Fatal("want a second cohort refused")
		}
	})

	t.Run("auth timeout", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, bps.Options{AuthTimeout: 50 * time.Millisecond})
		e.subscribe()
		pub, overlay := e.publisher()
		expectDone(t, pub)
		eventually(t, func() bool { return e.bl.blocklisted(overlay) })
		if got := e.counters().AuthTimeout; got != 1 {
			t.Fatalf("auth_timeout %d, want 1", got)
		}
	})

	t.Run("inactivity deadline", func(t *testing.T) {
		t.Parallel()
		e := newEnv(t, bps.Options{InactivityTimeout: 100 * time.Millisecond})
		sub := e.subscribe()
		pub, _ := e.publisher()
		e.publish(pub, bps.KindAuth, 0, nil)
		expectDone(t, sub)
		expectDone(t, pub)
		if _, ok := e.broker.Counters(bps.CohortSpec{Topic: e.topic, Principal: e.principal}); ok {
			t.Fatal("cohort not reclaimed")
		}
	})
}

func TestVerify(t *testing.T) {
	t.Parallel()
	e := newEnv(t, bps.Options{})
	challenge := make([]byte, 32)
	copy(challenge, "challenge")

	c := e.chunk(e.signer, bps.KindData, challenge, 9, []byte("x"))
	if err := bps.Verify(bps.KindData, challenge, 9, c, e.topic, e.principal); err != nil {
		t.Fatal(err)
	}
	other := make([]byte, 20)
	for _, err := range []error{
		bps.Verify(bps.KindData, challenge, 9, c, e.topic, other),
		bps.Verify(bps.KindData, challenge[:31], 9, c, e.topic, e.principal),
		bps.Verify(bps.KindAuth, challenge, 9, c, e.topic, e.principal),
		bps.Verify(bps.KindData, challenge, 9, c, make([]byte, 32), e.principal),
		bps.Verify(bps.KindData, challenge, 9, c[:10], e.topic, e.principal),
	} {
		if !errors.Is(err, bps.ErrInvalidSOC) {
			t.Fatalf("got %v, want invalid soc", err)
		}
	}
}
