// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/coreos/go-semver/semver"
	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var errFlushStreamReset = errors.New("flush stream reset")

// flushStream is a broker-side stream whose peer is driven by the test: the
// peer's frames are written into a pipe the broker reads, and the broker's
// writes land in a buffer — or, once stalled, block as on a transport whose
// peer has stopped reading, until the stream is reset or closed.
type flushStream struct {
	r  *io.PipeReader
	pw *io.PipeWriter

	mu      sync.Mutex
	out     bytes.Buffer
	stall   bool
	stalled chan struct{} // closed when a write blocks
	unblock chan struct{} // closed by Reset or Close

	reset, closed          chan struct{}
	stallOnce, unblockOnce sync.Once
	resetOnce, closedOnce  sync.Once
}

func newFlushStream() *flushStream {
	r, pw := io.Pipe()
	return &flushStream{
		r: r, pw: pw,
		stalled: make(chan struct{}),
		unblock: make(chan struct{}),
		reset:   make(chan struct{}),
		closed:  make(chan struct{}),
	}
}

func (s *flushStream) Read(p []byte) (int, error) { return s.r.Read(p) }

func (s *flushStream) Write(p []byte) (int, error) {
	s.mu.Lock()
	if !s.stall {
		defer s.mu.Unlock()
		return s.out.Write(p)
	}
	s.mu.Unlock()
	s.stallOnce.Do(func() { close(s.stalled) })
	<-s.unblock
	return 0, errFlushStreamReset
}

func (s *flushStream) end() {
	s.unblockOnce.Do(func() { close(s.unblock) })
	_ = s.r.Close()
}

func (s *flushStream) Reset() error {
	s.resetOnce.Do(func() { close(s.reset) })
	s.end()
	return nil
}

func (s *flushStream) Close() error {
	s.closedOnce.Do(func() { close(s.closed) })
	s.end()
	return nil
}

func (s *flushStream) FullClose() error                  { return s.Close() }
func (s *flushStream) Headers() p2p.Headers              { return nil }
func (s *flushStream) ResponseHeaders() p2p.Headers      { return nil }
func (s *flushStream) Version() (*semver.Version, error) { return nil, nil }

// stallWrites makes every later broker write block.
func (s *flushStream) stallWrites() {
	s.mu.Lock()
	s.stall = true
	s.mu.Unlock()
}

// frames returns what the broker has written so far, after the Ack.
func (s *flushStream) frames(t *testing.T) []*pb.Broadcast {
	t.Helper()
	s.mu.Lock()
	r := protobuf.NewReader(bytes.NewReader(append([]byte{}, s.out.Bytes()...)))
	s.mu.Unlock()
	var ack pb.Ack
	if err := r.ReadMsg(&ack); err != nil {
		t.Fatal(err)
	}
	var fs []*pb.Broadcast
	for {
		var f pb.Broadcast
		if err := r.ReadMsg(&f); err != nil {
			return fs
		}
		fs = append(fs, &f)
	}
}

// flushEnv runs the broker's handler over flushStreams.
type flushEnv struct {
	t     *testing.T
	svc   *Service
	admin crypto.Signer
	spec  *pb.CohortSpec
}

func newFlushEnv(t *testing.T, o Options) *flushEnv {
	t.Helper()
	svc := New(nil, nil, true, log.Noop, o)
	t.Cleanup(func() { _ = svc.Close() })
	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(key)
	principal, err := signer.EthereumAddress()
	if err != nil {
		t.Fatal(err)
	}
	topic := make([]byte, swarm.HashSize)
	copy(topic, "bps-flush-topic")
	return &flushEnv{t: t, svc: svc, admin: signer,
		spec: &pb.CohortSpec{Topic: topic, Binding: pb.TopicBinding_FEED_TOPIC, Principal: principal.Bytes()}}
}

func newSigner(t *testing.T) (crypto.Signer, []byte) {
	t.Helper()
	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	signer := crypto.NewDefaultSigner(key)
	addr, err := signer.EthereumAddress()
	if err != nil {
		t.Fatal(err)
	}
	return signer, addr.Bytes()
}

// peer is the test's end of one stream.
type peer struct {
	e         *flushEnv
	st        *flushStream
	w         protobuf.Writer
	signer    crypto.Signer
	challenge []byte
	done      chan struct{} // the handler returned
}

// join attaches a stream declaring identity and returns once the Ack is written.
func (e *flushEnv) join(signer crypto.Signer, identity []byte) *peer {
	e.t.Helper()
	st := newFlushStream()
	p := &peer{e: e, st: st, w: protobuf.NewWriter(st.pw), signer: signer, done: make(chan struct{})}
	go func() {
		defer close(p.done)
		_ = e.svc.handler(context.Background(), p2p.Peer{Address: swarm.RandAddress(e.t)}, st)
	}()
	go func() { _ = p.w.WriteMsg(&pb.Join{Cohort: e.spec, Identity: identity}) }()
	deadline := time.Now().Add(waitFlush)
	for {
		st.mu.Lock()
		var ack pb.Ack
		err := protobuf.NewReader(bytes.NewReader(st.out.Bytes())).ReadMsg(&ack)
		st.mu.Unlock()
		if err == nil {
			if ack.Status != pb.Status_OK {
				e.t.Fatalf("join: %s", ack.Status)
			}
			p.challenge = ack.Challenge
			return p
		}
		if time.Now().After(deadline) {
			e.t.Fatal("no ack")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// send signs a chunk of kind at index under the stream's challenge and writes it.
func (p *peer) send(kind pb.Kind, index uint64, payload []byte) error {
	chunkID, err := id(kind, p.e.spec.Topic, p.challenge, index)
	if err != nil {
		return err
	}
	ch, err := cac.New(payload)
	if err != nil {
		return err
	}
	c, err := soc.New(chunkID, ch).Sign(p.signer)
	if err != nil {
		return err
	}
	return p.w.WriteMsg(&pb.Broadcast{Soc: c.Data(), Kind: kind, Challenge: p.challenge, Index: index})
}

func (p *peer) mustSend(kind pb.Kind, index uint64, payload []byte) {
	p.e.t.Helper()
	if err := p.send(kind, index, payload); err != nil {
		p.e.t.Fatal(err)
	}
}

func (e *flushEnv) roster(admin *peer, index uint64, addrs ...[]byte) {
	e.t.Helper()
	payload, err := RosterPayload(addrs)
	if err != nil {
		e.t.Fatal(err)
	}
	admin.mustSend(pb.Kind_ROSTER, index, payload)
}

func (e *flushEnv) counters() Counters {
	e.t.Helper()
	key, err := e.spec.Marshal()
	if err != nil {
		e.t.Fatal(err)
	}
	e.svc.mtx.Lock()
	co := e.svc.cohorts[string(key)]
	e.svc.mtx.Unlock()
	if co == nil {
		e.t.Fatal("no live cohort")
	}
	return co.counters.snapshot()
}

const waitFlush = 5 * time.Second

func waitClosed(t *testing.T, c <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-c:
	case <-time.After(waitFlush):
		t.Fatalf("%s: timed out", what)
	}
}

func notClosed(t *testing.T, c <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-c:
		t.Fatalf("%s: unexpected", what)
	default:
	}
}

func eventuallyTrue(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitFlush)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestFlushDeadline covers the flush deadline of a stream being closed — by
// an EOS or by a revocation — on its write side, where the peer has stopped
// reading and the broker's write blocks, and on its read side, where the peer
// goes on sending or never closes.
func TestFlushDeadline(t *testing.T) {
	t.Parallel()
	const flush = 300 * time.Millisecond

	t.Run("write: an EOS resets a stream whose peer stopped reading, at the deadline", func(t *testing.T) {
		t.Parallel()
		e := newFlushEnv(t, Options{FlushTimeout: flush})
		admin := e.join(e.admin, e.spec.Principal)
		admin.mustSend(pb.Kind_AUTH, 0, nil)
		_, other := newSigner(t)
		sub := e.join(nil, other)
		sub.st.stallWrites()

		// the update blocks the subscriber's writer mid-write, outside any flush
		admin.mustSend(pb.Kind_DATA, 0, []byte("stuck"))
		waitClosed(t, sub.st.stalled, "the write did not block")

		start := time.Now()
		admin.mustSend(pb.Kind_EOS, 0, nil)
		waitClosed(t, sub.st.reset, "the stalled stream was not reset")
		if elapsed := time.Since(start); elapsed < flush*3/4 {
			t.Fatalf("reset after %v, before the flush deadline %v", elapsed, flush)
		}
		waitClosed(t, sub.done, "the stalled stream's handler did not return")
		notClosed(t, sub.st.closed, "the stalled stream closed")

		// the sender, which reads, takes its EOS and is closed, not reset
		waitClosed(t, admin.st.closed, "the admin's stream was not closed")
		notClosed(t, admin.st.reset, "the admin's stream reset")
		waitClosed(t, admin.done, "the admin's handler did not return")
		fs := admin.st.frames(t)
		if len(fs) == 0 || fs[len(fs)-1].Kind != pb.Kind_EOS {
			t.Fatalf("the admin's stream did not end on its EOS: %v", fs)
		}
	})

	t.Run("write: a revocation resets a publisher stream whose peer stopped reading, at the deadline", func(t *testing.T) {
		t.Parallel()
		e := newFlushEnv(t, Options{FlushTimeout: flush})
		admin := e.join(e.admin, e.spec.Principal)
		admin.mustSend(pb.Kind_AUTH, 0, nil)
		seatSigner, seatAddr := newSigner(t)
		e.roster(admin, 0, seatAddr)
		seat := e.join(seatSigner, seatAddr)
		eventuallyTrue(t, func() bool { return len(seat.st.frames(t)) > 0 }) // the cue
		seat.mustSend(pb.Kind_AUTH, 0, nil)
		eventuallyTrue(t, func() bool {
			fs := admin.st.frames(t)
			return len(fs) > 0 && fs[len(fs)-1].Kind == pb.Kind_ROSTER
		})
		seat.st.stallWrites()

		admin.mustSend(pb.Kind_DATA, 0, []byte("stuck"))
		waitClosed(t, seat.st.stalled, "the write did not block")

		start := time.Now()
		e.roster(admin, 1)
		waitClosed(t, seat.st.reset, "the stalled revoked stream was not reset")
		if elapsed := time.Since(start); elapsed < flush*3/4 {
			t.Fatalf("reset after %v, before the flush deadline %v", elapsed, flush)
		}
		waitClosed(t, seat.done, "the revoked stream's handler did not return")
		notClosed(t, admin.st.reset, "the admin's stream reset")
	})

	t.Run("read: a revoked peer that keeps sending is closed, not reset, its frames dropped", func(t *testing.T) {
		t.Parallel()
		e := newFlushEnv(t, Options{FlushTimeout: 10 * flush})
		admin := e.join(e.admin, e.spec.Principal)
		admin.mustSend(pb.Kind_AUTH, 0, nil)
		seatSigner, seatAddr := newSigner(t)
		e.roster(admin, 0, seatAddr)
		seat := e.join(seatSigner, seatAddr)
		eventuallyTrue(t, func() bool { return len(seat.st.frames(t)) > 0 })
		seat.mustSend(pb.Kind_AUTH, 0, nil)
		eventuallyTrue(t, func() bool {
			fs := admin.st.frames(t)
			return len(fs) > 0 && fs[len(fs)-1].Kind == pb.Kind_ROSTER
		})

		// the seat floods its stream with updates until it is closed
		flooding := make(chan struct{})
		go func() {
			defer close(flooding)
			for i := uint64(0); ; i++ {
				if seat.send(pb.Kind_DATA, i, []byte("flood")) != nil {
					return
				}
			}
		}()
		eventuallyTrue(t, func() bool { return e.counters().Retransmit == 0 && len(admin.st.frames(t)) > 2 })

		start := time.Now()
		e.roster(admin, 1)
		waitClosed(t, seat.st.closed, "the revoked stream was not closed")
		if elapsed := time.Since(start); elapsed >= 10*flush {
			t.Fatalf("closed after %v: the flood held it to the deadline", elapsed)
		}
		notClosed(t, seat.st.reset, "the revoked stream reset")
		waitClosed(t, seat.done, "the revoked stream's handler did not return")
		waitClosed(t, flooding, "the flood did not stop")

		c := e.counters()
		if c.WrongStream != 0 || c.InvalidSOC != 0 {
			t.Fatalf("a revoked peer's frames punished: %+v", c)
		}
		fs := seat.st.frames(t)
		if last := fs[len(fs)-1]; last.Kind != pb.Kind_ROSTER || last.Index != 1 {
			t.Fatalf("the revoked stream did not end on the reduced roster: %s %d", last.Kind, last.Index)
		}
	})

	t.Run("read: a silent peer that never closes its side does not hold an ended stream", func(t *testing.T) {
		t.Parallel()
		e := newFlushEnv(t, Options{FlushTimeout: 10 * flush})
		admin := e.join(e.admin, e.spec.Principal)
		admin.mustSend(pb.Kind_AUTH, 0, nil)
		_, other := newSigner(t)
		sub := e.join(nil, other)

		start := time.Now()
		admin.mustSend(pb.Kind_EOS, 0, nil)
		waitClosed(t, sub.st.closed, "the ended stream was not closed")
		waitClosed(t, sub.done, "the ended stream's handler did not return")
		if elapsed := time.Since(start); elapsed >= 10*flush {
			t.Fatalf("closed after %v: the silent peer held it to the deadline", elapsed)
		}
		notClosed(t, sub.st.reset, "the ended stream reset")
		fs := sub.st.frames(t)
		if len(fs) != 1 || fs[0].Kind != pb.Kind_EOS {
			t.Fatalf("the ended stream got %v, want the EOS alone", fs)
		}
	})
}
