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
	if n := sub.st.resets.Load() + sub.st.closes.Load(); n != 1 {
		t.Fatalf("the stream was ended %d times (resets %d, closes %d)", n, sub.st.resets.Load(), sub.st.closes.Load())
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
