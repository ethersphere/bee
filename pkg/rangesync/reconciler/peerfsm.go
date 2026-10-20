// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reconciler

import (
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
)

// The per-peer state machine (§8.1). It does no I/O: the reconciler fires
// its events, under r.mu, as Status sends, sessions, stripes and fetches
// end, and passes the time.
//
//	[*] --> Probing: peer connected
//	Probing --> Unsupported: rangesync stream not supported
//	Probing --> Idle: Status exchanged, root relation known
//	Idle --> Idle: Status with equal digest
//	Idle --> Reconciling: digest differs (initiator) or timer
//	Reconciling --> Fetching: session done, wants queued
//	Reconciling --> Striped: large difference
//	Striped --> Fetching: stripe done
//	Fetching --> Idle: wants delivered or released
//	Reconciling --> Idle: Busy / error (backoff)
//	Idle --> [*]: disconnected or neighborhoods disjoint
//	Unsupported --> [*]: disconnected
//
// The reconciler adds four kinds of transition to the diagram:
//   - Idle --> Striped: a stripe session with the peer starts. The stripe
//     worker plans a striped range across every neighbor that covers it
//     (§7.4, §7.5), not only the peer whose session found the difference, so
//     a stripe session starts from Idle. A session hands its large ranges to
//     the stripe worker and ends (Reconciling --> Fetching), so the wiring
//     does not take Reconciling --> Striped: the peer cannot serve a stripe
//     while its own session runs (MaxConcurrentSessionsPerPeer = 1).
//   - Fetching --> Striped: a stripe session may also start while the wants
//     of the peer's last session are fetched. The fetch uses its own stream,
//     not the session slot. The peer returns to Idle once both have ended,
//     which the machine tracks itself (peerFSM.fetching): the fetch ending
//     first is Striped --> Striped, and stripe done goes to Fetching only
//     while the fetch runs. With no round fetch pending, stripe done goes
//     to Idle: the stripe's own wants are fetched while the peer is Striped.
//   - Reconciling --> Idle without a backoff: the session did not run for a
//     cause on this node (eventAborted), which says nothing about the peer.
//   - any state --> [*] on a disconnect.
//
// [*] is stateRemoved. A peer that leaves because the neighborhoods became
// disjoint stays connected and keeps receiving Statuses (gossip.go); it gets
// a new machine when a Status shows the neighborhoods overlap again.
type fsmState int

const (
	stateProbing fsmState = iota + 1
	stateUnsupported
	stateIdle
	stateReconciling
	stateFetching
	stateStriped
	stateRemoved
)

var stateNames = map[fsmState]string{
	stateProbing:     "probing",
	stateUnsupported: "unsupported",
	stateIdle:        "idle",
	stateReconciling: "reconciling",
	stateFetching:    "fetching",
	stateStriped:     "striped",
	stateRemoved:     "removed",
}

// String returns the state's name.
func (s fsmState) String() string {
	if n, ok := stateNames[s]; ok {
		return n
	}
	return "unknown"
}

// fsmEvent is an event of the per-peer state machine.
type fsmEvent int

const (
	// eventUnsupported: a Status send reported rangesync.ErrProtocolUnsupported.
	eventUnsupported fsmEvent = iota + 1
	// eventExchanged: a Status of this node reached the peer and the peer's
	// Status is known, so the relation of the two roots is known.
	eventExchanged
	// eventEqualDigest: a round found the peer's digest equal to this node's.
	eventEqualDigest
	// eventReconcile: a session with the peer starts.
	eventReconcile
	// eventSessionDone: the session completed; its wants are being fetched.
	eventSessionDone
	// eventLargeDiff: the session moves to a striped transfer of a large
	// difference with the peer.
	eventLargeDiff
	// eventStripe: a stripe session with the peer starts (transfer or
	// verification, §7.4).
	eventStripe
	// eventStripeDone: the stripe session ended; its wants are being fetched.
	eventStripeDone
	// eventFetched: the wants were delivered or released.
	eventFetched
	// eventBusy: the session was answered Busy (isBusy).
	eventBusy
	// eventError: the session failed.
	eventError
	// eventAborted: the session did not run for a cause on this node
	// (localCause): its session cap, its index rebuilding, a stale scope.
	eventAborted
	// eventDisjoint: the peer is not eligible (isEligible): the
	// neighborhoods are disjoint or the Mappings differ.
	eventDisjoint
	// eventDisconnected: the peer left the topology.
	eventDisconnected
)

var eventNames = map[fsmEvent]string{
	eventUnsupported:  "unsupported",
	eventExchanged:    "exchanged",
	eventEqualDigest:  "equal-digest",
	eventReconcile:    "reconcile",
	eventSessionDone:  "session-done",
	eventLargeDiff:    "large-diff",
	eventStripe:       "stripe",
	eventStripeDone:   "stripe-done",
	eventFetched:      "fetched",
	eventBusy:         "busy",
	eventError:        "error",
	eventAborted:      "aborted",
	eventDisjoint:     "disjoint",
	eventDisconnected: "disconnected",
}

// String returns the event's name.
func (e fsmEvent) String() string {
	if n, ok := eventNames[e]; ok {
		return n
	}
	return "unknown"
}

// transitions is the transition table: the next state by state and event.
// An event a state does not list is not a transition of that state.
var transitions = map[fsmState]map[fsmEvent]fsmState{
	stateProbing: {
		eventUnsupported:  stateUnsupported,
		eventExchanged:    stateIdle,
		eventDisconnected: stateRemoved,
	},
	stateUnsupported: {
		eventDisconnected: stateRemoved,
	},
	stateIdle: {
		eventEqualDigest:  stateIdle,
		eventReconcile:    stateReconciling,
		eventStripe:       stateStriped,
		eventDisjoint:     stateRemoved,
		eventDisconnected: stateRemoved,
	},
	stateReconciling: {
		eventSessionDone:  stateFetching,
		eventLargeDiff:    stateStriped,
		eventBusy:         stateIdle,
		eventError:        stateIdle,
		eventAborted:      stateIdle,
		eventDisconnected: stateRemoved,
	},
	stateStriped: {
		eventStripeDone:   stateFetching, // stateIdle with no round fetch (fire)
		eventFetched:      stateStriped,
		eventDisconnected: stateRemoved,
	},
	stateFetching: {
		eventFetched:      stateIdle,
		eventStripe:       stateStriped,
		eventDisconnected: stateRemoved,
	},
}

// transition returns the state e leads to from s, and false when e is not a
// transition of s.
func transition(s fsmState, e fsmEvent) (fsmState, bool) {
	next, ok := transitions[s][e]
	return next, ok
}

const (
	// sessionBackoffBase is the wait before the next session with a peer
	// after its second Busy or failed session in a row; each further one
	// doubles it. The first is retried by the next round, which runs at
	// least MinRoundInterval after the last (scheduler.go).
	sessionBackoffBase = rangesync.MinRoundInterval
	// maxSessionBackoff bounds the wait of a peer that stays Busy or keeps
	// failing; it is still retried that often.
	maxSessionBackoff = 5 * time.Minute
)

// peerFSM is a peer's state machine, with the backoff of its sessions.
type peerFSM struct {
	state fsmState
	// fetching holds while the wants of the peer's last round session are
	// fetched (from eventSessionDone to eventFetched), in Fetching and in a
	// stripe session that started meanwhile.
	fetching bool
	// failures counts the Busy or failed sessions in a row (the streak);
	// retryAt is when the next session may start.
	failures int
	retryAt  time.Time
}

// newPeerFSM returns the machine of a peer that connected.
func newPeerFSM() peerFSM {
	return peerFSM{state: stateProbing}
}

// fire applies e at now and reports whether it was a transition of the
// current state; any other event leaves the machine unchanged. Busy and
// error extend the streak and set the backoff (sessionBackoff). A completed
// session or an equal digest ends the streak, and so does a quiet period of
// maxSessionBackoff past the last backoff: a streak counts answers in a row,
// not over the peer's lifetime. An aborted session leaves it as it is. A
// completed session starts the fetch of its wants and eventFetched ends it;
// stripe done leads to Fetching only while that fetch runs, else to Idle.
func (m *peerFSM) fire(e fsmEvent, now time.Time) bool {
	next, ok := transition(m.state, e)
	if !ok {
		return false
	}
	switch e {
	case eventBusy, eventError:
		if now.Sub(m.retryAt) > maxSessionBackoff {
			m.failures = 0
		}
		m.failures++
		m.retryAt = now.Add(sessionBackoff(m.failures))
	case eventSessionDone:
		m.failures, m.retryAt = 0, time.Time{}
		m.fetching = true
	case eventEqualDigest:
		m.failures, m.retryAt = 0, time.Time{}
	case eventFetched:
		m.fetching = false
	case eventStripeDone:
		if !m.fetching {
			next = stateIdle
		}
	}
	m.state = next
	return true
}

// canReconcile reports whether a session with the peer may start at now:
// it is Idle and its backoff has passed.
func (m *peerFSM) canReconcile(now time.Time) bool {
	return m.state == stateIdle && !now.Before(m.retryAt)
}

// sessionBackoff returns the wait before the next session with a peer after
// n Busy or failed sessions in a row: none after the first (the next round
// retries it), then sessionBackoffBase doubling up to maxSessionBackoff.
func sessionBackoff(n int) time.Duration {
	if n <= 1 {
		return 0
	}
	return backoff(sessionBackoffBase, maxSessionBackoff, n-1)
}

// statusRetry returns the wait before a Status send is retried after n
// failed sends in a row: StatusMinInterval doubling up to StatusMaxInterval
// (the heartbeat), so that a failing or black-holed peer is not retried
// every StatusMinInterval.
func statusRetry(n int) time.Duration {
	return backoff(StatusMinInterval, StatusMaxInterval, n)
}

// backoff returns base doubled n-1 times, at most limit.
func backoff(base, limit time.Duration, n int) time.Duration {
	d := base
	for i := 1; i < n && d < limit; i++ {
		d *= 2
	}
	return min(d, limit)
}
