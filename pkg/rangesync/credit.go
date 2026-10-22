// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"container/list"
	"errors"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// The per-peer fetch credit (T4.7 part 4). A peer's share of the fetch rate
// refills only with the chunks it delivered, so that a peer that lists junk
// runs out of it whatever path it uses (small fresh junk, root moves, resets,
// unknown-batch stamps; FetchWithResult):
//
//   - Asking a peer for an ID costs one unit of its credit, taken with the
//     Want's stream slot, before the open and the wait for the node-wide
//     fetch rate. With less credit than candidates, the Want asks for what
//     the credit covers and the rest waits for the next Want of the call, by
//     when this one settled. With none, the fetch requests nothing more from
//     the peer and fails at once (ErrFetchCredit, FetchResult.NoCredit),
//     taking no fetch rate, claim or open: its IDs are free for other peers'
//     fetches, and the caller can ask other peers for them.
//   - A chunk the peer delivers and this node stores gives back its unit and
//     earns one more (creditStoredRefund), up to creditMax: the credit of a
//     peer that delivers more than half of what it is asked for grows, so an
//     honest peer, which delivers nearly all, is never held up, and the
//     junk a peer is asked for stays below the chunks it stored plus its
//     initial credit and probes.
//   - Neutral, the unit back and nothing more: a valid delivery of a chunk
//     the reserve held when it arrived (another protocol stored it
//     meanwhile: honest work that gave this node nothing it lacked), a
//     delivery whose stamp could not be checked for a cause on this node,
//     the IDs this node did not request after all (another claim, the
//     reserve, the want set), and the IDs of a Want this node ended (its
//     context, Close, a local error) or that did not start (its stream did
//     not open).
//   - Nothing back: the IDs the peer left missing, those of a Want it reset
//     or stalled, and deliveries this node rejected (a stamp invalid
//     anywhere, of an unknown or expired batch, older than the reserve's).
//   - A peer with no credit left is asked for one ID, a probe, once per
//     creditProbeInterval, so that an honest peer that ran out (a newcomer
//     the pool had nothing for, a peer that lost Wants) can earn its way
//     back: a delivered probe gives two, and each Want after it doubles.
//
// The book keeps the credit per overlay across disconnects, so that a
// reconnect, which is cheap, renews nothing. A new overlay starts with
// creditInitial drawn from a node-wide pool of creditPoolBurst IDs that
// regains creditInitial per creditPoolInterval, and gets its first probe
// creditProbeInterval after it was first asked: what a new overlay buys an
// attacker (about 2^depth hashes) is a share of that pool, bounded for all
// new overlays together. The book keeps at most maxCreditPeers overlays and
// forgets the one asked least recently first; it remembers the balance of
// the last maxCreditPeers overlays it forgot in debt (below creditInitial),
// and one of them asked again resumes it instead of drawing from the pool,
// its probe clock started over. So evicting and returning is no cheaper than
// staying: a debt is forgotten only after maxCreditPeers other overlays
// were asked for IDs and maxCreditPeers more were forgotten in debt, and
// then the overlay is a new one, which a fresh overlay is too.
const (
	// creditInitial is what a new overlay starts with, at most: one full
	// Want, drawn from the pool.
	creditInitial = MaxWantIDs
	// creditMax is the most credit a peer holds: two Wants on each of the
	// maxFetchStreamsOut streams this node opens to it, so that a peer at
	// full credit is never held up, and keeps two full Wants after losing
	// two (a reset, a radius increase).
	creditMax = 2 * maxFetchStreamsOut * MaxWantIDs
	// creditStoredRefund is what a stored chunk gives back: its unit and
	// one more.
	creditStoredRefund = 2
	// creditPoolBurst and creditPoolInterval set the node-wide pool of
	// initial credit: the initial credit of 32 new overlays at once (a
	// neighborhood when the node starts), and one more per minute.
	creditPoolBurst    = 32 * creditInitial
	creditPoolInterval = time.Minute
	// creditProbeInterval is how often a peer with no credit left is asked
	// for one ID.
	creditProbeInterval = 10 * time.Second
	// maxCreditPeers bounds the overlays the book keeps, and the debts of
	// those it forgot it remembers (about 150 bytes each, 1.2 MB in all).
	maxCreditPeers = 4096
)

// ErrFetchCredit fails a fetch from a peer whose fetch credit is spent
// (credit.go): it requests nothing more from the peer and fails at once,
// without an open, fetch rate or claim, and reports the IDs it did not
// request (FetchResult.NoCredit). The peer is not scored: ask other peers
// for the IDs.
var ErrFetchCredit = errors.New("rangesync: the peer's fetch credit is spent")

// creditBook keeps the fetch credit of each overlay (the rules above). A nil
// book grants every request. It is safe for concurrent use.
type creditBook struct {
	mu       sync.Mutex
	capacity int
	pool     *rate.Limiter
	// peers holds the overlays the book keeps, order them by when they were
	// last asked, the most recent first; debts and debtOrder the balances
	// of those it forgot in debt, the most recently forgotten first.
	peers     map[string]*list.Element // values are *peerCredit
	order     *list.List
	debts     map[string]*list.Element // values are *peerCredit
	debtOrder *list.List
}

// peerCredit is the credit of one overlay: the IDs it may be asked for now,
// and when it may be asked for one ID if it has none.
type peerCredit struct {
	key     string
	balance int
	probeAt time.Time
}

// newCreditBook returns a book that keeps at most capacity overlays, and the
// debts of capacity overlays it forgot, with a full pool.
func newCreditBook(capacity int) *creditBook {
	return &creditBook{
		capacity:  capacity,
		pool:      rate.NewLimiter(rate.Limit(float64(creditInitial)/creditPoolInterval.Seconds()), creditPoolBurst),
		peers:     make(map[string]*list.Element),
		order:     list.New(),
		debts:     make(map[string]*list.Element),
		debtOrder: list.New(),
	}
}

// take takes up to n units of peer's credit at now and returns how many it
// took: all n, what is left, or, with nothing left, one when a probe is due.
// An overlay the book does not keep resumes its debt if the book remembers
// it, and else draws its initial credit from the pool. take makes peer the
// most recently asked overlay, and may forget the least recently asked one.
func (b *creditBook) take(peer swarm.Address, n int, now time.Time) int {
	if b == nil {
		return n
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	k := peer.ByteString()
	e, ok := b.peers[k]
	if ok {
		b.order.MoveToFront(e)
	} else {
		e = b.order.PushFront(b.arriveLocked(k, now))
		b.peers[k] = e
		for b.order.Len() > b.capacity {
			b.forgetLocked(b.order.Back())
		}
	}
	pc := e.Value.(*peerCredit)
	if pc.balance == 0 && !now.Before(pc.probeAt) {
		pc.balance, pc.probeAt = 1, now.Add(creditProbeInterval)
	}
	got := min(n, pc.balance)
	pc.balance -= got
	return got
}

// arriveLocked returns the credit of the overlay with key k, which the book
// does not keep, at now: its debt if the book remembers it, else what the
// pool has of creditInitial. Its first probe is due creditProbeInterval
// later. b.mu must be held.
func (b *creditBook) arriveLocked(k string, now time.Time) *peerCredit {
	pc := &peerCredit{key: k, probeAt: now.Add(creditProbeInterval)}
	if e, ok := b.debts[k]; ok {
		pc.balance = e.Value.(*peerCredit).balance
		b.debtOrder.Remove(e)
		delete(b.debts, k)
		return pc
	}
	pc.balance = min(creditInitial, int(b.pool.TokensAt(now)+1e-9))
	if pc.balance > 0 {
		b.pool.AllowN(now, pc.balance)
	}
	return pc
}

// forgetLocked forgets the overlay in e and remembers its balance if it is in
// debt, forgetting the oldest debt past the capacity. b.mu must be held.
func (b *creditBook) forgetLocked(e *list.Element) {
	pc := e.Value.(*peerCredit)
	b.order.Remove(e)
	delete(b.peers, pc.key)
	if pc.balance >= creditInitial {
		return // back, it gets at most this: nothing to remember
	}
	b.debts[pc.key] = b.debtOrder.PushFront(pc)
	for b.debtOrder.Len() > b.capacity {
		last := b.debtOrder.Back()
		b.debtOrder.Remove(last)
		delete(b.debts, last.Value.(*peerCredit).key)
	}
}

// give adds n units to peer's credit, kept or remembered as a debt, up to
// creditMax. A peer the book neither keeps nor remembers gets nothing.
func (b *creditBook) give(peer swarm.Address, n int) {
	if b == nil || n <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	k := peer.ByteString()
	e, ok := b.peers[k]
	if !ok {
		if e, ok = b.debts[k]; !ok {
			return
		}
	}
	pc := e.Value.(*peerCredit)
	pc.balance = min(pc.balance+n, creditMax)
}

// credit returns peer's credit and whether the book keeps it.
func (b *creditBook) credit(peer swarm.Address) (int, bool) {
	if b == nil {
		return 0, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.peers[peer.ByteString()]
	if !ok {
		return 0, false
	}
	return e.Value.(*peerCredit).balance, true
}

// size returns the number of overlays the book keeps and the number of
// debts it remembers.
func (b *creditBook) size() (peers, debts int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.order.Len(), b.debtOrder.Len()
}

// FetchCredit returns peer's fetch credit (credit.go): the IDs this node
// would ask it for now, without a probe; 0 for a peer it has not asked for
// IDs, or forgot. The credit grows with the chunks the peer delivered and
// shrinks with what it was asked for and did not deliver, and with its Wants
// in flight.
func (s *Service) FetchCredit(peer swarm.Address) int {
	c, _ := s.credits.credit(peer)
	return c
}
