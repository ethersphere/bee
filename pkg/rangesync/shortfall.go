// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/wantset"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// The fetch shortfall (proposal §9, "bogus wants count against the peer"). A
// peer is asked only for IDs it listed, so what it does not deliver counts
// against it (FetchWithResult, judgeShortfall):
//
//   - The shortfall of a fetch is the IDs requested in Wants the peer closed
//     without delivering them (missing), and its deliveries whose stamp is
//     invalid on every node (stampInvalid).
//   - A missing ID that left the peer's root since the fetch started is
//     excused when the root moved by a plausible radius increase
//     (plausibleStep), one such move per peer per rootExcuseInterval. The
//     fetch requests no ID outside the peer's root by its last Status.
//   - Blatant: when at least half of at least blatantMinIDs requested IDs
//     fall short, the fetch is a violation at once (scored, disconnected).
//   - Otherwise a missing ID counts only when the peer left it missing
//     before, within shortfallWindow. An honest peer does not list again an
//     ID it evicted or replaced (its index commits the change first), so
//     evictions, batch expiries and in-place rewrites (SOC, GSOC, re-stamped
//     uploads) do not count, and junk listed again does. Invalid stamps count
//     at once.
//   - An offense, a count above shortfallTolerance of the IDs requested, is
//     reported to the PeerScorer without a disconnect (the fetch's error
//     wraps ErrFetchShortfall); a second offense within shortfallWindow is a
//     violation.
//   - Nothing is judged without a Status of the peer (it disconnected), after
//     the fetch reported a violation, or while the Service closes.
const (
	// shortfallMin and shortfallRatio set the shortfall a fetch tolerates
	// from its peer (shortfallTolerance): at most shortfallMin IDs, or
	// 1/shortfallRatio of the IDs requested, whichever is more, as the
	// reconciler's stripe verification tolerates.
	shortfallMin   = 4
	shortfallRatio = 100
	// blatantMinIDs is the fewest requested IDs a blatant shortfall, half of
	// them, needs.
	blatantMinIDs = 64
	// shortfallWindow is how long a missing ID is remembered for its peer,
	// and how long an offense makes the next one a violation: the
	// reconciler's ScoreDecayInterval.
	shortfallWindow = 10 * time.Minute
	// maxMissingPerPeer bounds the missing IDs remembered for one peer.
	maxMissingPerPeer = wantset.MaxClaimsPerPeer
	// maxExcuseStep is the largest radius increase (bits) a root move may
	// make to excuse the missing IDs that left the root.
	maxExcuseStep = 2
	// rootExcuseInterval is how often one peer's root move may excuse
	// missing IDs: radius increases are rare.
	rootExcuseInterval = 10 * time.Minute
	// maxFetchResets and fetchResetInterval bound the fetch streams of one
	// peer that fail without a delivery (a reset instead of a close): a
	// burst of maxFetchResets, then one per fetchResetInterval. Past them a
	// failure is a violation.
	maxFetchResets     = 4
	fetchResetInterval = time.Minute
)

// ErrFetchShortfall is wrapped by the error of a fetch whose peer fell short
// (the shortfall rules above), and by the reason reported to the
// PeerScorer. Without ErrInvalidMessage it is a first offense, and the peer
// was not disconnected.
var ErrFetchShortfall = errors.New("rangesync: fetch shortfall")

// shortfallTolerance returns the shortfall a fetch of requested IDs
// tolerates from its peer.
func shortfallTolerance(requested int) int {
	return max(shortfallMin, requested/shortfallRatio)
}

// plausibleStep reports whether a peer's root moved from from to to by a
// radius increase of at most maxExcuseStep bits.
func plausibleStep(from, to fingerprint.Range) bool {
	return to.Bits > from.Bits && to.Bits-from.Bits <= maxExcuseStep && from.ContainsRange(to)
}

// shortfall is what one FetchWithResult call holds against its peer.
type shortfall struct {
	requested int              // IDs requested in Wants
	missing   []fingerprint.ID // requested and not delivered when the peer closed the stream
	invalid   int              // delivered with a stamp invalid on every node (stampInvalid)

	// start is the peer's root when the fetch started; hasStart reports
	// that the peer had a Status then.
	start    fingerprint.Range
	hasStart bool

	// The classification of missing[:classified] against root, the peer's
	// root at the last judgment: counted of them count. asked records that
	// the excuse of the move from start to root was asked for, excused the
	// answer.
	root       fingerprint.Range
	classified int
	counted    int
	asked      bool
	excused    bool
}

// moved reports whether id left the peer's root, now root, since the fetch
// started.
func (sf *shortfall) moved(id fingerprint.ID, root fingerprint.Range) bool {
	return sf.hasStart && sf.start.ContainsID(id) && !root.ContainsID(id)
}

// classify brings the classification of the missing IDs up to date with
// root, the peer's root now, and returns the number that count. The first
// time a missing ID left the root it asks excuse whether the move from the
// start is excused, when it is a plausible step.
func (sf *shortfall) classify(root fingerprint.Range, excuse func(from, to fingerprint.Range) bool) int {
	if !root.Equal(sf.root) {
		sf.root, sf.classified, sf.counted, sf.asked, sf.excused = root, 0, 0, false, false
	}
	for _, id := range sf.missing[sf.classified:] {
		if sf.moved(id, root) {
			if !sf.asked {
				sf.asked = true
				sf.excused = plausibleStep(sf.start, root) && excuse(sf.start, root)
			}
			if sf.excused {
				continue
			}
		}
		sf.counted++
	}
	sf.classified = len(sf.missing)
	return sf.counted
}

// counting returns the missing IDs that count, as the last classify
// classified them.
func (sf *shortfall) counting() []fingerprint.ID {
	if !sf.excused {
		return sf.missing
	}
	out := make([]fingerprint.ID, 0, sf.counted)
	for _, id := range sf.missing {
		if !sf.moved(id, sf.root) {
			out = append(out, id)
		}
	}
	return out
}

// judgeShortfall judges the shortfall sf of a fetch from peer by the rules
// above and returns the error the fetch ends with, or nil. end marks the
// fetch's last judgment: only it checks the missing IDs against those the
// peer left missing before and records them, and scores an offense; the
// judgments after each Want stop a blatant shortfall at once.
func (s *Service) judgeShortfall(peer swarm.Address, sf *shortfall, end bool) error {
	select {
	case <-s.ctx.Done():
		return nil
	default:
	}
	remote, ok := s.LastStatus(peer)
	if !ok {
		return nil // the peer disconnected: its root is unknown
	}
	now := s.now()
	excuse := func(from, to fingerprint.Range) bool { return s.shortfalls.excuse(peer, from, to, now) }
	short := sf.classify(remote.Root, excuse) + sf.invalid
	if sf.requested >= blatantMinIDs && 2*short >= sf.requested {
		return s.initiatorViolation(peer, fmt.Errorf("%w: %w: %d of %d requested ids not delivered or with an invalid stamp (%d)", ErrInvalidMessage, ErrFetchShortfall, short, sf.requested, sf.invalid))
	}
	if !end {
		return nil
	}
	counted := s.shortfalls.relisted(peer, sf.counting(), now) + sf.invalid
	if counted <= shortfallTolerance(sf.requested) {
		return nil
	}
	reason := fmt.Errorf("%w: %d of %d requested ids missing again or with an invalid stamp (%d)", ErrFetchShortfall, counted, sf.requested, sf.invalid)
	if s.shortfalls.offend(peer, now) {
		return s.initiatorViolation(peer, fmt.Errorf("%w: %w, again within %s", ErrInvalidMessage, reason, shortfallWindow))
	}
	s.scorer.Fail(peer, reason)
	s.logger.Debug("fetch shortfall, peer scored", "peer_address", peer, "error", reason)
	return reason
}

// shortfallBook keeps what the shortfall rules remember per peer: the IDs it
// left missing and its last offense, forgotten on a disconnect, and its last
// excused root move, kept across reconnects so that a reconnect does not
// renew it. It is safe for concurrent use.
type shortfallBook struct {
	mu      sync.Mutex
	peers   map[string]*peerShortfall
	excuses map[string]rootExcuse
}

// peerShortfall is what the book keeps for one peer.
type peerShortfall struct {
	// missing holds when each ID was last left missing; order holds them
	// in the order they were first left missing, for pruning.
	missing     map[fingerprint.ID]time.Time
	order       []fingerprint.ID
	lastOffense time.Time
}

// rootExcuse is the root move of a peer, from from to to, excused at at.
type rootExcuse struct {
	from, to fingerprint.Range
	at       time.Time
}

func newShortfallBook() *shortfallBook {
	return &shortfallBook{peers: make(map[string]*peerShortfall), excuses: make(map[string]rootExcuse)}
}

// peerLocked returns the entry of peer, created if missing. b.mu must be
// held.
func (b *shortfallBook) peerLocked(peer swarm.Address) *peerShortfall {
	p, ok := b.peers[peer.ByteString()]
	if !ok {
		p = &peerShortfall{missing: make(map[fingerprint.ID]time.Time)}
		b.peers[peer.ByteString()] = p
	}
	return p
}

// excuse reports whether the move of peer's root from from to to excuses the
// IDs that left it, at now: the first move within rootExcuseInterval does,
// and so does the same move again (the concurrent fetches of one radius
// increase).
func (b *shortfallBook) excuse(peer swarm.Address, from, to fingerprint.Range, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for k, e := range b.excuses {
		if now.Sub(e.at) >= rootExcuseInterval {
			delete(b.excuses, k)
		}
	}
	if e, ok := b.excuses[peer.ByteString()]; ok {
		return e.from.Equal(from) && e.to.Equal(to)
	}
	b.excuses[peer.ByteString()] = rootExcuse{from: from, to: to, at: now}
	return true
}

// relisted records ids as left missing by peer at now, and returns how many
// of them it had left missing before, within shortfallWindow. It keeps at
// most maxMissingPerPeer IDs per peer, the oldest dropped first.
func (b *shortfallBook) relisted(peer swarm.Address, ids []fingerprint.ID, now time.Time) int {
	if len(ids) == 0 {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	p := b.peerLocked(peer)
	n := 0
	for _, id := range ids {
		at, ok := p.missing[id]
		if ok && now.Sub(at) < shortfallWindow {
			n++
		}
		if !ok {
			p.order = append(p.order, id)
		}
		p.missing[id] = now
	}
	for len(p.order) > 0 {
		id := p.order[0]
		if at := p.missing[id]; now.Sub(at) < shortfallWindow && len(p.missing) <= maxMissingPerPeer {
			break
		}
		p.order = p.order[1:]
		delete(p.missing, id)
	}
	return n
}

// offend records an offense of peer at now and reports whether it repeats
// one within shortfallWindow.
func (b *shortfallBook) offend(peer swarm.Address, now time.Time) (repeat bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	p := b.peerLocked(peer)
	repeat = !p.lastOffense.IsZero() && now.Sub(p.lastOffense) < shortfallWindow
	p.lastOffense = now
	return repeat
}

// clear forgets the missing IDs and the offenses of peer.
func (b *shortfallBook) clear(peer swarm.Address) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.peers, peer.ByteString())
}
