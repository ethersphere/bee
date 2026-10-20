// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"

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
//     (plausibleStep), one such move per peer per rootExcuseInterval, and
//     only for the fetches that were running when it was first excused
//     (the concurrent fetches of one radius increase), so a root that swings
//     back and forth gets one move excused. The fetch requests no ID outside
//     the peer's root by its last Status.
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
//   - Nothing is judged, and nothing is recorded for the peer, without a
//     Status of the peer (it disconnected), after a violation of the peer
//     (this fetch's, or another's within violationGrace), or while the
//     Service closes. A violation marks the peer only while it has a
//     Status, and a peer marked already is not reported again.
//   - Apart from the shortfall, a fetch stream that fails with IDs still
//     undelivered (a reset by the peer; not this node ending the fetch nor
//     the fetch timeout) takes one of the peer's maxFetchResets, then one
//     per fetchResetInterval; past them it is a violation. Only a stream
//     whose Want claimed IDs is charged: a stream that fails to open has
//     taken no fetch rate and claimed nothing, and is never charged. The
//     requester opens at most maxFetchStreamsOut streams to a peer, so one
//     transient failure takes at most that many.
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
	// peer that fail with IDs undelivered (a reset instead of a close): a
	// burst of maxFetchResets, then one per fetchResetInterval. Past them a
	// failure is a violation.
	maxFetchResets     = 4
	fetchResetInterval = time.Minute
	// violationGrace is how long after a violation of a peer the shortfall
	// rules and the reset budget leave it alone: it is being disconnected,
	// and its other fetches fail with it.
	violationGrace = time.Minute
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

	// start is the peer's root when the fetch started at startedAt;
	// hasStart reports that the peer had a Status then.
	start     fingerprint.Range
	hasStart  bool
	startedAt time.Time

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
	// read after the Status: a disconnect from here on is forgetIfGone's
	now := s.now()
	if !ok || s.shortfalls.violated(peer, now) {
		return nil // the peer disconnected (its root is unknown), or is being disconnected
	}
	excuse := func(from, to fingerprint.Range) bool { return s.shortfalls.excuse(peer, from, to, sf.startedAt, now) }
	short := sf.classify(remote.Root, excuse) + sf.invalid
	if sf.requested >= blatantMinIDs && 2*short >= sf.requested {
		return s.initiatorViolation(peer, fmt.Errorf("%w: %w: %d of %d requested ids not delivered or with an invalid stamp (%d)", ErrInvalidMessage, ErrFetchShortfall, short, sf.requested, sf.invalid))
	}
	if !end {
		return nil
	}
	// a disconnect meanwhile clears what the book records for the peer
	defer s.forgetIfGone(peer)
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

// chargeReset takes one of peer's fetch resets, for a fetch stream that
// failed with claimed IDs undelivered, and reports whether its budget was
// spent (maxFetchResets, then one per fetchResetInterval). Nothing is
// charged for a peer whose Status is gone (it disconnected) or that is being
// disconnected for a violation.
func (s *Service) chargeReset(peer swarm.Address) (spent bool) {
	_, ok := s.LastStatus(peer)
	// read after the Status: a disconnect from here on is forgetIfGone's
	now := s.now()
	if !ok || s.shortfalls.violated(peer, now) {
		return false
	}
	defer s.forgetIfGone(peer)
	return !s.fetchResets.take(peer, now)
}

// markViolation marks peer as reported for a violation, for violationGrace,
// and reports whether it was marked already: reported within the grace, so
// that the caller does not report it again (one violation, one score; the
// peer is being disconnected). A peer whose Status is gone (it
// disconnected, and the disconnect hook cleared its mark) is not marked, so
// that no mark outlives its connection and exempts the next one.
func (s *Service) markViolation(peer swarm.Address) (marked bool) {
	_, ok := s.LastStatus(peer)
	// read after the Status: a disconnect from here on is forgetIfGone's
	if s.shortfalls.markViolation(peer, s.now(), ok) {
		return true
	}
	if ok {
		s.forgetIfGone(peer)
	}
	return false
}

// forgetIfGone clears what the shortfall rules and the reset budget keep for
// peer when its Status is gone: it disconnected while they recorded, after
// the disconnect hook cleared them. A Status that handleStatus records after
// the disconnect hook (an accepted race, see there) hides the disconnect:
// what was recorded then is kept until the peer's next disconnect, or until
// it expires (a mark after violationGrace, missing IDs and an offense after
// shortfallWindow, reset charges at one per fetchResetInterval).
func (s *Service) forgetIfGone(peer swarm.Address) {
	if _, ok := s.LastStatus(peer); !ok {
		s.shortfalls.clear(peer)
		s.fetchResets.clear(peer)
	}
}

// shortfallBook keeps what the shortfall rules remember per peer: the IDs it
// left missing, its last offense and its last violation, forgotten on a
// disconnect, and its last excused root move, kept across reconnects so that
// a reconnect does not renew it. It is safe for concurrent use.
type shortfallBook struct {
	mu         sync.Mutex
	peers      map[string]*peerShortfall
	excuses    map[string]rootExcuse
	violations map[string]time.Time
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
	return &shortfallBook{
		peers:      make(map[string]*peerShortfall),
		excuses:    make(map[string]rootExcuse),
		violations: make(map[string]time.Time),
	}
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
// IDs that left it, for a fetch that started at started, at now: the first
// move within rootExcuseInterval does, and the same move does again for a
// fetch that started before it was first excused (the concurrent fetches of
// one radius increase), not for a later one (the same move after the root
// moved back).
func (b *shortfallBook) excuse(peer swarm.Address, from, to fingerprint.Range, started, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for k, e := range b.excuses {
		if now.Sub(e.at) >= rootExcuseInterval {
			delete(b.excuses, k)
		}
	}
	if e, ok := b.excuses[peer.ByteString()]; ok {
		return e.from.Equal(from) && e.to.Equal(to) && !started.After(e.at)
	}
	b.excuses[peer.ByteString()] = rootExcuse{from: from, to: to, at: now}
	return true
}

// markViolation reports whether peer was reported for a violation within
// violationGrace before now and, if it was not and set is true, records that
// it was reported at now.
func (b *shortfallBook) markViolation(peer swarm.Address, now time.Time, set bool) (marked bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.violatedLocked(peer, now) {
		return true
	}
	if set {
		b.violations[peer.ByteString()] = now
	}
	return false
}

// violated reports whether peer was reported for a violation within
// violationGrace before now.
func (b *shortfallBook) violated(peer swarm.Address, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.violatedLocked(peer, now)
}

// violatedLocked is violated with b.mu held.
func (b *shortfallBook) violatedLocked(peer swarm.Address, now time.Time) bool {
	at, ok := b.violations[peer.ByteString()]
	if ok && now.Sub(at) >= violationGrace {
		delete(b.violations, peer.ByteString())
		return false
	}
	return ok
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

// clear forgets the missing IDs, the offenses and the violation of peer.
func (b *shortfallBook) clear(peer swarm.Address) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.peers, peer.ByteString())
	delete(b.violations, peer.ByteString())
}

// peerBudget is a budget of events per peer on an injected clock: burst
// events back to back, then one per every. It is safe for concurrent use.
type peerBudget struct {
	mu    sync.Mutex
	every time.Duration
	burst int
	peers map[string]*rate.Limiter
}

func newPeerBudget(every time.Duration, burst int) *peerBudget {
	return &peerBudget{every: every, burst: burst, peers: make(map[string]*rate.Limiter)}
}

// take takes one event of peer's budget at now and reports whether there
// was one.
func (b *peerBudget) take(peer swarm.Address, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	l, ok := b.peers[peer.ByteString()]
	if !ok {
		l = rate.NewLimiter(rate.Every(b.every), b.burst)
		b.peers[peer.ByteString()] = l
	}
	return l.AllowN(now, 1)
}

// clear forgets peer's budget.
func (b *peerBudget) clear(peer swarm.Address) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.peers, peer.ByteString())
}
