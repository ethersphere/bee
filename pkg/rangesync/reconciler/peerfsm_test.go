// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
)

// transition is one edge of the per-peer state machine.
type transition struct {
	from  reconciler.PeerState
	event reconciler.PeerEvent
	to    reconciler.PeerState
}

// spec81 holds every transition of the §8.1 state diagram. Removed is its
// final state ([*]).
var spec81 = []transition{
	{reconciler.StateProbing, reconciler.EventUnsupported, reconciler.StateUnsupported},
	{reconciler.StateProbing, reconciler.EventExchanged, reconciler.StateIdle},
	{reconciler.StateIdle, reconciler.EventEqualDigest, reconciler.StateIdle},
	{reconciler.StateIdle, reconciler.EventReconcile, reconciler.StateReconciling},
	{reconciler.StateReconciling, reconciler.EventSessionDone, reconciler.StateFetching},
	{reconciler.StateReconciling, reconciler.EventLargeDiff, reconciler.StateStriped},
	{reconciler.StateStriped, reconciler.EventStripeDone, reconciler.StateFetching},
	{reconciler.StateFetching, reconciler.EventFetched, reconciler.StateIdle},
	{reconciler.StateReconciling, reconciler.EventBusy, reconciler.StateIdle},
	{reconciler.StateReconciling, reconciler.EventError, reconciler.StateIdle},
	{reconciler.StateIdle, reconciler.EventDisconnected, reconciler.StateRemoved},
	{reconciler.StateIdle, reconciler.EventDisjoint, reconciler.StateRemoved},
	{reconciler.StateUnsupported, reconciler.EventDisconnected, reconciler.StateRemoved},
}

// beyond81 holds the transitions the reconciler adds to §8.1: a stripe
// session with an idle neighbor (§7.4 assigns stripes to neighbors that run
// no session) or with one whose last wants are being fetched (the fetch
// does not take the session slot), and a disconnect in every state.
var beyond81 = []transition{
	{reconciler.StateIdle, reconciler.EventStripe, reconciler.StateStriped},
	{reconciler.StateFetching, reconciler.EventStripe, reconciler.StateStriped},
	{reconciler.StateProbing, reconciler.EventDisconnected, reconciler.StateRemoved},
	{reconciler.StateReconciling, reconciler.EventDisconnected, reconciler.StateRemoved},
	{reconciler.StateFetching, reconciler.EventDisconnected, reconciler.StateRemoved},
	{reconciler.StateStriped, reconciler.EventDisconnected, reconciler.StateRemoved},
}

// TestPeerFSMTransitions fires every transition of §8.1 (and the ones the
// reconciler adds) on a machine in its source state.
func TestPeerFSMTransitions(t *testing.T) {
	t.Parallel()

	if got := reconciler.NewPeerFSM().State(); got != reconciler.StateProbing {
		t.Fatalf("a connected peer starts in %v, want %v", got, reconciler.StateProbing)
	}
	for _, tc := range append(append([]transition(nil), spec81...), beyond81...) {
		t.Run(tc.from.String()+"/"+tc.event.String(), func(t *testing.T) {
			t.Parallel()
			m := reconciler.PeerFSMIn(tc.from)
			if !m.Fire(tc.event, time.Unix(0, 0)) {
				t.Fatalf("%v rejected in %v", tc.event, tc.from)
			}
			if got := m.State(); got != tc.to {
				t.Fatalf("%v --%v--> %v, want %v", tc.from, tc.event, got, tc.to)
			}
		})
	}
}

// TestPeerFSMRejects checks that the table is exactly the listed
// transitions: every other event leaves the machine where it is, and
// Removed is final.
func TestPeerFSMRejects(t *testing.T) {
	t.Parallel()

	allowed := make(map[[2]int]bool)
	for _, tc := range append(append([]transition(nil), spec81...), beyond81...) {
		allowed[[2]int{int(tc.from), int(tc.event)}] = true
	}
	for _, s := range reconciler.PeerStates {
		for _, e := range reconciler.PeerEvents {
			_, ok := reconciler.Transition(s, e)
			if want := allowed[[2]int{int(s), int(e)}]; ok != want {
				t.Fatalf("%v in %v: allowed %v, want %v", e, s, ok, want)
			}
			if ok {
				continue
			}
			m := reconciler.PeerFSMIn(s)
			if m.Fire(e, time.Unix(0, 0)) || m.State() != s {
				t.Fatalf("%v in %v moved the machine to %v", e, s, m.State())
			}
		}
	}
}

// TestPeerFSMPullsyncOnly: a peer that does not speak rangesync ends in
// Unsupported, where only a disconnect moves it.
func TestPeerFSMPullsyncOnly(t *testing.T) {
	t.Parallel()

	m := reconciler.NewPeerFSM()
	now := time.Unix(0, 0)
	if !m.Fire(reconciler.EventUnsupported, now) || m.State() != reconciler.StateUnsupported {
		t.Fatalf("state %v, want %v", m.State(), reconciler.StateUnsupported)
	}
	if m.CanReconcile(now) {
		t.Fatal("an unsupported peer can reconcile")
	}
	if !m.Fire(reconciler.EventDisconnected, now) || m.State() != reconciler.StateRemoved {
		t.Fatalf("state %v after the disconnect, want %v", m.State(), reconciler.StateRemoved)
	}
}

// TestPeerFSMBackoff: the first Busy or failed session is retried at once
// (by the next round); each further one in a row doubles the wait from the
// base up to the cap; a completed session resets it.
func TestPeerFSMBackoff(t *testing.T) {
	t.Parallel()

	start := time.Unix(1_000_000, 0)
	m := reconciler.NewPeerFSM()
	m.Fire(reconciler.EventExchanged, start)
	if !m.CanReconcile(start) {
		t.Fatal("an idle peer cannot reconcile")
	}

	now := start
	want := []time.Duration{0, rangesync.MinRoundInterval, 2 * rangesync.MinRoundInterval, 4 * rangesync.MinRoundInterval}
	for d := want[len(want)-1]; d < reconciler.MaxSessionBackoff; {
		d = min(2*d, reconciler.MaxSessionBackoff)
		want = append(want, d)
	}
	want = append(want, reconciler.MaxSessionBackoff)
	for i, wait := range want {
		ev := reconciler.EventBusy
		if i%2 == 1 {
			ev = reconciler.EventError
		}
		if !m.Fire(reconciler.EventReconcile, now) || m.CanReconcile(now) {
			t.Fatalf("answer %d: session not started", i)
		}
		if !m.Fire(ev, now) || m.State() != reconciler.StateIdle {
			t.Fatalf("answer %d: state %v after %v", i, m.State(), ev)
		}
		if got := m.RetryAt().Sub(now); got != wait {
			t.Fatalf("answer %d in a row: retry in %v, want %v", i+1, got, wait)
		}
		if wait > 0 && m.CanReconcile(now.Add(wait-time.Nanosecond)) {
			t.Fatalf("answer %d: reconciles before the backoff passed", i+1)
		}
		now = now.Add(wait)
		if !m.CanReconcile(now) {
			t.Fatalf("answer %d: cannot reconcile after the backoff", i+1)
		}
	}

	// a completed session resets the backoff
	m.Fire(reconciler.EventReconcile, now)
	m.Fire(reconciler.EventSessionDone, now)
	m.Fire(reconciler.EventFetched, now)
	m.Fire(reconciler.EventReconcile, now)
	m.Fire(reconciler.EventBusy, now)
	if got := m.RetryAt(); got.After(now) {
		t.Fatalf("retry at %v after a completed session and one Busy, want now", got)
	}
}

// TestPeerFSMBusyOnlyFromReconciling: a backoff applies only to the
// session that answered; a stripe or fetch in flight is not interrupted by
// it, and no session starts while the peer is not Idle.
func TestPeerFSMBusyOnlyFromReconciling(t *testing.T) {
	t.Parallel()

	now := time.Unix(0, 0)
	for _, s := range []reconciler.PeerState{reconciler.StateFetching, reconciler.StateStriped, reconciler.StateProbing, reconciler.StateRemoved} {
		m := reconciler.PeerFSMIn(s)
		if m.Fire(reconciler.EventBusy, now) {
			t.Fatalf("Busy accepted in %v", s)
		}
		if m.CanReconcile(now) {
			t.Fatalf("can reconcile in %v", s)
		}
	}
}

// TestStatusRetry: a Status that could not be sent is retried after
// StatusMinInterval, doubling with each failure in a row up to
// StatusMaxInterval.
func TestStatusRetry(t *testing.T) {
	t.Parallel()

	for n, want := range map[int]time.Duration{
		1: rangesync.StatusMinInterval,
		2: 2 * rangesync.StatusMinInterval,
		3: 4 * rangesync.StatusMinInterval,
		4: 8 * rangesync.StatusMinInterval,
		5: rangesync.StatusMaxInterval,
		9: rangesync.StatusMaxInterval,
	} {
		if got := reconciler.StatusRetry(n); got != want {
			t.Fatalf("retry after %d failures: %v, want %v", n, got, want)
		}
	}
}
