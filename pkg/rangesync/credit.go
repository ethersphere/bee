// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"container/list"
	"errors"
	"hash/maphash"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/storer"
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
//     earns one more (creditStoredRefund), up to creditMax, and one excuse
//     (below), up to creditMax: the credit of a peer that delivers more than
//     half of what it is asked for grows, so an honest peer is never held
//     up, and the junk a peer is asked for stays below the chunks it stored
//     plus its initial credit and probes, and its excuses. The chunks it
//     stores need not be new content: a peer with a batch can mint them, as
//     overwrites of one stamp index with ever newer timestamps, so the
//     credit bounds the junk only of a peer without a batch.
//   - Neutral, the unit back and nothing more: a valid delivery of a chunk
//     the reserve held when it arrived (another protocol stored it
//     meanwhile: honest work that gave this node nothing it lacked), a
//     valid delivery older than the one the reserve holds
//     (storage.ErrOverwriteNewerChunk: a peer that holds an older version
//     delivers it honestly, §7.7, and a peer with a batch earns more by
//     delivering the newer one), a delivery whose stamp could not be
//     checked for a cause on this node, the IDs this node did not request
//     after all (another claim, the reserve, the want set), and the IDs of
//     a Want this node ended (its context, Close, a local error) or that did
//     not start (its stream did not open).
//   - A delivery whose stamp depends on the chain state this node has seen
//     (chainStateRejected: an unknown or expired batch, an index past the
//     batch depth this node knows) is neutral when this node held the ID
//     itself (removals: it removed it from its reserve since, the batch
//     having expired here first) and else when the peer has an excuse left,
//     which it spends. A peer's excuses grow only with the chunks it stores,
//     so that a peer without a batch cannot mint them, and such a stamp of a
//     batch this node knew cannot be told from one made up (the batch store
//     forgets an expired batch, and checks the index before the owner).
//   - Nothing back: the IDs the peer left missing, those of a Want it reset
//     or stalled, and the other deliveries this node rejected (a stamp
//     invalid anywhere, a chain-state rejection the peer has no excuse
//     for).
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
// new overlays together, and no excuse. The book keeps at most
// maxCreditPeers overlays; past it, it forgets the one with the least
// credit, the one asked least recently among equals, so that the new
// overlays an attacker has asked, which hold at most their share of the
// pool, do not push out the neighbors that earned more (a synced neighbor
// is asked for nothing for long). It remembers the credit of the last
// maxCreditPeers overlays it forgot, and one of them asked again resumes it,
// debt or credit, instead of drawing from the pool, its probe clock started
// over. So evicting and returning is no cheaper than staying, and an honest
// neighbor forgotten comes back with its credit: a debt is forgotten only
// after maxCreditPeers more overlays were forgotten, and then the overlay
// is a new one, which a fresh overlay is too.
const (
	// creditInitial is what a new overlay starts with, at most: one full
	// Want, drawn from the pool.
	creditInitial = MaxWantIDs
	// creditMax is the most credit a peer holds: two Wants on each of the
	// maxFetchStreamsOut streams this node opens to it, so that a peer at
	// full credit is never held up, and keeps two full Wants after losing
	// two (a reset, a radius increase). It also bounds a peer's excuses.
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
	// maxCreditPeers bounds the overlays the book keeps, and the credit of
	// those it forgot it remembers (about 150 bytes each, 1.2 MB in all).
	maxCreditPeers = 4096
	// maxRemovals bounds the IDs removed from the reserve that the record
	// of removals keeps, the most recent ones, in two generations (8 bytes
	// each and the map's overhead, about 1 MB in all).
	maxRemovals = 1 << 16
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
	// last asked, the most recent first; forgotten and forgotOrder the
	// credit of those it forgot, the most recently forgotten first.
	peers       map[string]*list.Element // values are *peerCredit
	order       *list.List
	forgotten   map[string]*list.Element // values are *peerCredit
	forgotOrder *list.List
}

// peerCredit is the credit of one overlay: the IDs it may be asked for now,
// the chain-state rejections of its deliveries the book still excuses, and
// when it may be asked for one ID if it has no credit.
type peerCredit struct {
	key     string
	balance int
	excuses int
	probeAt time.Time
}

// newCreditBook returns a book that keeps at most capacity (at least one)
// overlays, and the credit of capacity overlays it forgot, with a full pool.
func newCreditBook(capacity int) *creditBook {
	return &creditBook{
		capacity:    max(capacity, 1),
		pool:        rate.NewLimiter(rate.Limit(float64(creditInitial)/creditPoolInterval.Seconds()), creditPoolBurst),
		peers:       make(map[string]*list.Element),
		order:       list.New(),
		forgotten:   make(map[string]*list.Element),
		forgotOrder: list.New(),
	}
}

// take takes up to n units of peer's credit at now and returns how many it
// took: all n, what is left, or, with nothing left, one when a probe is due.
// An overlay the book does not keep resumes its credit if the book
// remembers it, and else draws its initial credit from the pool. take makes
// peer the most recently asked overlay, and may forget another one, the one
// with the least credit (evictLocked).
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
			b.forgetLocked(b.evictLocked())
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
// does not keep, at now: what it had if the book remembers it, else what the
// pool has of creditInitial, with no excuse. Its first probe is due
// creditProbeInterval later. b.mu must be held.
func (b *creditBook) arriveLocked(k string, now time.Time) *peerCredit {
	if e, ok := b.forgotten[k]; ok {
		pc := e.Value.(*peerCredit)
		b.forgotOrder.Remove(e)
		delete(b.forgotten, k)
		pc.probeAt = now.Add(creditProbeInterval)
		return pc
	}
	pc := &peerCredit{key: k, probeAt: now.Add(creditProbeInterval)}
	// the rounding may ask for a unit more than the pool holds, which
	// AllowN then refuses: the pool grants only what it is charged
	n := min(creditInitial, int(b.pool.TokensAt(now)+1e-9))
	for n > 0 && !b.pool.AllowN(now, n) {
		n--
	}
	pc.balance = n
	return pc
}

// evictLocked returns the overlay the book forgets next: the one with the
// least credit, the least recently asked among equals, but never the most
// recently asked one (the overlay take is asking). b.mu must be held, and
// the book must keep at least two overlays.
func (b *creditBook) evictLocked() *list.Element {
	victim := b.order.Back()
	for e := victim; e != b.order.Front(); e = e.Prev() {
		if e.Value.(*peerCredit).balance < victim.Value.(*peerCredit).balance {
			victim = e
		}
		if victim.Value.(*peerCredit).balance == 0 {
			break // none has less
		}
	}
	return victim
}

// forgetLocked forgets the overlay in e and remembers its credit, forgetting
// the oldest remembered one past the capacity. b.mu must be held.
func (b *creditBook) forgetLocked(e *list.Element) {
	pc := e.Value.(*peerCredit)
	b.order.Remove(e)
	delete(b.peers, pc.key)
	b.forgotten[pc.key] = b.forgotOrder.PushFront(pc)
	for b.forgotOrder.Len() > b.capacity {
		last := b.forgotOrder.Back()
		b.forgotOrder.Remove(last)
		delete(b.forgotten, last.Value.(*peerCredit).key)
	}
}

// lookupLocked returns the credit of peer, kept or remembered, and whether
// the book keeps it; nil if the book neither keeps nor remembers it. b.mu
// must be held.
func (b *creditBook) lookupLocked(peer swarm.Address) (*peerCredit, bool) {
	k := peer.ByteString()
	if e, ok := b.peers[k]; ok {
		return e.Value.(*peerCredit), true
	}
	if e, ok := b.forgotten[k]; ok {
		return e.Value.(*peerCredit), false
	}
	return nil, false
}

// give adds n units to peer's credit, kept or remembered, up to creditMax. A
// peer the book neither keeps nor remembers gets nothing.
func (b *creditBook) give(peer swarm.Address, n int) {
	if b == nil || n <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if pc, _ := b.lookupLocked(peer); pc != nil {
		pc.balance = min(pc.balance+n, creditMax)
	}
}

// stored gives peer, kept or remembered, what a chunk it delivered and this
// node stored earns: creditStoredRefund units and one excuse, each up to
// creditMax.
func (b *creditBook) stored(peer swarm.Address) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if pc, _ := b.lookupLocked(peer); pc != nil {
		pc.balance = min(pc.balance+creditStoredRefund, creditMax)
		pc.excuses = min(pc.excuses+1, creditMax)
	}
}

// excuse spends one of peer's excuses, if it has one left, on a delivery
// rejected for the chain state this node has seen, and gives back its unit;
// it reports whether it did.
func (b *creditBook) excuse(peer swarm.Address) bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	pc, _ := b.lookupLocked(peer)
	if pc == nil || pc.excuses == 0 {
		return false
	}
	pc.excuses--
	pc.balance = min(pc.balance+1, creditMax)
	return true
}

// credit returns peer's credit, kept or remembered (0 if neither), and
// whether the book keeps it.
func (b *creditBook) credit(peer swarm.Address) (int, bool) {
	if b == nil {
		return 0, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	pc, kept := b.lookupLocked(peer)
	if pc == nil {
		return 0, false
	}
	return pc.balance, kept
}

// size returns the number of overlays the book keeps and the number of
// forgotten ones whose credit it remembers.
func (b *creditBook) size() (peers, forgotten int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.order.Len(), b.forgotOrder.Len()
}

// FetchCredit returns peer's fetch credit (credit.go): the IDs this node
// would ask it for now, without a probe; 0 for a peer it has not asked for
// IDs, or whose credit it no longer remembers. The credit grows with the
// chunks the peer delivered and shrinks with what it was asked for and did
// not deliver, and with its Wants in flight.
func (s *Service) FetchCredit(peer swarm.Address) int {
	c, _ := s.credits.credit(peer)
	return c
}

// removals records the IDs removed from this node's reserve, the most recent
// maxRemovals of them, each until it excuses one rejected delivery (take): a
// delivery of an ID this node held itself, rejected for the chain state (a
// batch that expired here first), says nothing about the peer. Keys are
// hashes of the IDs under a seed of this node's, which a peer cannot aim
// at. It is safe for concurrent use.
type removals struct {
	mu   sync.Mutex
	seed maphash.Seed
	// cur holds the most recent removals, at most maxRemovals/2; prev those
	// before them.
	cur, prev map[uint64]struct{}
}

// newRemovals returns an empty record of removals with a random seed.
func newRemovals() *removals {
	return &removals{seed: maphash.MakeSeed(), cur: make(map[uint64]struct{})}
}

// key returns the key of id in the record.
func (r *removals) key(id fingerprint.ID) uint64 { return maphash.Bytes(r.seed, id[:]) }

// add records ids as removed.
func (r *removals) add(ids []storer.ChunkID) {
	if len(ids) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range ids {
		if len(r.cur) >= maxRemovals/2 {
			r.prev, r.cur = r.cur, make(map[uint64]struct{}, maxRemovals/2)
		}
		r.cur[r.key(fingerprint.NewID(c.Address, c.StampHash))] = struct{}{}
	}
}

// take reports whether id is recorded as removed, and forgets it, so that
// each removal excuses at most one delivery.
func (r *removals) take(id fingerprint.ID) bool {
	k := r.key(id)
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, m := range []map[uint64]struct{}{r.cur, r.prev} {
		if _, ok := m[k]; ok {
			delete(m, k)
			return true
		}
	}
	return false
}

// removalObserver is the reserve observer Start registers: the Indexer, to
// which it passes every call, and the record of the IDs the reserve removed
// (removals), which the fetch credit reads. It records a transaction's
// removes after the Indexer applied them, in memory only, so it keeps the
// Indexer's side of the storer.ReserveObserver contract.
type removalObserver struct {
	indexer  *Indexer
	removals *removals
}

var _ storer.ReserveObserver = removalObserver{}

// Commit implements storer.ReserveObserver.
func (o removalObserver) Commit(run func() (adds, removes []storer.ChunkID, err error)) error {
	var removed []storer.ChunkID
	err := o.indexer.Commit(func() (adds, removes []storer.ChunkID, err error) {
		adds, removes, err = run()
		removed = removes
		return adds, removes, err
	})
	o.removals.add(removed) // empty for a failed transaction
	return err
}

// RadiusChanged implements storer.ReserveObserver.
func (o removalObserver) RadiusChanged(radius uint8) { o.indexer.RadiusChanged(radius) }

// Reset implements storer.ReserveObserver.
func (o removalObserver) Reset() { o.indexer.Reset() }
