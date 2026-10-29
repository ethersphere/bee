// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bps

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/bps/pb"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// memberOf returns the cohort's member declaring identity.
func (e *flushEnv) memberOf(identity []byte) (*cohort, *member) {
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
	co.mtx.Lock()
	defer co.mtx.Unlock()
	for m := range co.attached {
		if bytes.Equal(m.identity, identity) {
			return co, m
		}
	}
	e.t.Fatal("no such member")
	return nil, nil
}

// TestRequestsEndAStreamOnce: any number of requests to close and to reset a
// stream, from any number of goroutines, end it exactly once — closed or
// reset, never both — and never block or panic.
func TestRequestsEndAStreamOnce(t *testing.T) {
	t.Parallel()
	e := newFlushEnv(t, Options{FlushTimeout: 300 * time.Millisecond})
	admin := e.join(e.admin, e.spec.Principal)
	admin.mustSend(pb.Kind_AUTH, 0, nil)
	_, other := newSigner(t)
	sub := e.join(nil, other)
	co, m := e.memberOf(other)

	var wg sync.WaitGroup
	for i := range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if i%2 == 0 {
				m.requestClose()
				return
			}
			co.mtx.Lock()
			m.requestReset()
			co.mtx.Unlock()
		}()
	}
	wg.Wait()
	waitClosed(t, sub.done, "the stream's handler did not return")
	if resets, closes := sub.st.ended(); resets+closes != 1 {
		t.Fatalf("the stream was ended %d times (resets %d, closes %d)", resets+closes, resets, closes)
	}
	// asking a stream that has ended changes nothing either
	m.requestClose()
	co.mtx.Lock()
	m.requestReset()
	co.mtx.Unlock()
	notClosed(t, admin.st.reset, "the admin's stream reset")
}

// TestLeave: when its streams end, a cohort leaves nothing behind — no member,
// no stream count of the peer, no cohort.
func TestLeave(t *testing.T) {
	t.Parallel()
	e := newFlushEnv(t, Options{})
	admin := e.join(e.admin, e.spec.Principal)
	admin.mustSend(pb.Kind_AUTH, 0, nil)
	_, other := newSigner(t)
	sub := e.join(nil, other)

	admin.mustSend(pb.Kind_EOS, 0, nil)
	waitClosed(t, admin.done, "the admin's handler did not return")
	waitClosed(t, sub.done, "the subscriber's handler did not return")
	e.svc.mtx.Lock()
	defer e.svc.mtx.Unlock()
	if len(e.svc.cohorts) != 0 || len(e.svc.peers) != 0 {
		t.Fatalf("left behind: %d cohorts, %d peers", len(e.svc.cohorts), len(e.svc.peers))
	}
}

// TestServiceClose: Close resets every stream and refuses later joins, and a
// second Close does nothing.
func TestServiceClose(t *testing.T) {
	t.Parallel()
	e := newFlushEnv(t, Options{})
	admin := e.join(e.admin, e.spec.Principal)
	admin.mustSend(pb.Kind_AUTH, 0, nil)
	_, other := newSigner(t)
	sub := e.join(nil, other)

	if err := e.svc.Close(); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Close(); err != nil {
		t.Fatal(err)
	}
	waitClosed(t, admin.st.reset, "the admin's stream was not reset")
	waitClosed(t, sub.st.reset, "the subscriber's stream was not reset")
	waitClosed(t, sub.done, "the subscriber's handler did not return")
	co, m, _, err := e.svc.join(swarm.RandAddress(t), &pb.Join{Cohort: e.spec, Identity: other})
	if !errors.Is(err, errSessionClosed) || co != nil || m != nil {
		t.Fatalf("join after Close: got %v, want %v", err, errSessionClosed)
	}
}

// TestSessionEnd: a session's read loop alone ends its stream, once — closed
// after a Close or an EOS, reset when the broker goes away — however many
// Closes are asked for, from however many goroutines, before or after.
func TestSessionEnd(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		end    func(t *testing.T, sess *session, broker *peer)
		err    error // the session's Err, when it is checked
		closed bool  // the stream is closed, not reset
	}{
		{"a Close closes the stream", func(_ *testing.T, sess *session, _ *peer) {
			var wg sync.WaitGroup
			for range 64 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_ = sess.Close()
				}()
			}
			wg.Wait()
		}, nil, true},
		{"an EOS closes the stream", func(t *testing.T, sess *session, broker *peer) {
			t.Helper()
			broker.mustSend(pb.Kind_EOS, 0, nil)
			if msg := <-sess.Messages(); msg.Kind != KindEOS {
				t.Errorf("got %v, want the EOS", msg.Kind)
			}
		}, ErrEndOfStream, true},
		{"the broker going away resets the stream", func(_ *testing.T, _ *session, broker *peer) {
			_ = broker.st.pw.Close()
		}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newFlushEnv(t, Options{})
			st := newFlushStream()
			w, r := protobuf.NewWriterAndReader(st)
			_, identity := newSigner(t)
			spec := CohortSpec{Topic: e.spec.Topic, Principal: e.spec.Principal}
			sess := newSession(e.svc, JoinRequest{Spec: spec, Identity: identity}, w, make([]byte, ChallengeSize))
			loop := make(chan struct{})
			go func() {
				defer close(loop)
				sess.readLoop(st, r)
			}()

			// the test is the broker: it writes the admin's frames, signed under
			// a publisher stream's challenge, into the session's stream
			broker := &peer{e: e, st: st, w: protobuf.NewWriter(st.pw), signer: e.admin, challenge: make([]byte, ChallengeSize)}
			tc.end(t, sess, broker)
			waitClosed(t, loop, "the read loop did not return")
			_ = sess.Close() // asked after the end, it changes nothing

			resets, closes := st.ended()
			if resets+closes != 1 || (closes == 1) != tc.closed {
				t.Fatalf("resets %d, closes %d; want one, closed %t", resets, closes, tc.closed)
			}
			waitClosed(t, sess.Done(), "Done was not closed")
			if _, ok := <-sess.Messages(); ok {
				t.Fatal("Messages was not closed")
			}
			if tc.err != nil && !errors.Is(sess.Err(), tc.err) {
				t.Fatalf("Err: got %v, want %v", sess.Err(), tc.err)
			}
		})
	}
}
