// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sim

import (
	"sort"
	"sync"
	"time"
)

// NodeHeal is one node's contribution to a heal episode.
type NodeHeal struct {
	Node   int
	Total  int
	Healed int
}

// Heal is one heal episode: the backfill that a storage-radius decrease opens
// up, measured from the moment the new radius was applied.
//
// Remaining is a legitimate outcome, not a failure. A node made responsible for
// bin R-1 only pulls that bin from a peer at PO >= R-1 (or the single bin po
// from a peer where radius-po <= 2), so a chunk whose only surviving holder is
// neither is unreachable by pull-sync and stays in the residue.
type Heal struct {
	ID         int
	FromRadius uint8
	ToRadius   uint8
	Total      int
	Healed     int
	Remaining  int
	Settled    bool
	HealSpanMs int64
	PerNode    []NodeHeal // only nodes with Total > 0
}

// healRec is the tracker's mutable record for one episode.
type healRec struct {
	id       int
	from, to uint8

	t0        time.Time
	lastPutAt time.Time

	// pending maps node index to the presence keys that node was missing when
	// the episode opened. A sync-sourced put strikes the key off.
	pending map[int]map[string]struct{}
	total   map[int]int
	healed  map[int]int

	totalAll  int
	healedAll int

	settled   bool
	published bool
	spanMs    int64

	done       chan struct{}
	doneClosed bool
}

// closeDone closes the episode's wait channel at most once. Must be called
// under the tracker mutex.
func (r *healRec) closeDone() {
	if r.doneClosed {
		return
	}
	r.doneClosed = true
	close(r.done)
}

// healTracker records heal episodes and strikes chunks off as they arrive.
//
// Like tracker it holds its own mutex and never calls back into the Network, so
// it is safe to read from Network.Snapshot while Network.mu is held.
type healTracker struct {
	now         func() time.Time
	settleAfter time.Duration

	mu      sync.Mutex
	seq     int
	records map[int]*healRec
	order   []int // episode IDs, oldest first
}

func newHealTracker(settleAfter time.Duration, now func() time.Time) *healTracker {
	return &healTracker{
		now:         now,
		settleAfter: settleAfter,
		records:     make(map[int]*healRec),
	}
}

// open registers a new episode. deficits maps node index to the set of presence
// keys that node is missing at the new radius; nodes with an empty set are kept
// out of the report entirely. An episode with nothing to do settles at once.
func (t *healTracker) open(from, to uint8, deficits map[int]map[string]struct{}) int {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.seq++
	rec := &healRec{
		id:      t.seq,
		from:    from,
		to:      to,
		t0:      t.now(),
		pending: make(map[int]map[string]struct{}, len(deficits)),
		total:   make(map[int]int, len(deficits)),
		healed:  make(map[int]int, len(deficits)),
		done:    make(chan struct{}),
	}
	for node, keys := range deficits {
		if len(keys) == 0 {
			continue
		}
		set := make(map[string]struct{}, len(keys))
		for k := range keys {
			set[k] = struct{}{}
		}
		rec.pending[node] = set
		rec.total[node] = len(set)
		rec.totalAll += len(set)
	}
	if rec.totalAll == 0 {
		rec.settled = true
		rec.closeDone()
	}
	t.records[rec.id] = rec
	t.order = append(t.order, rec.id)
	t.evictLocked()
	return rec.id
}

// evictLocked drops the oldest episodes beyond trackerRetain. Must be called
// under t.mu.
func (t *healTracker) evictLocked() {
	for len(t.order) > trackerRetain {
		id := t.order[0]
		t.order = t.order[1:]
		if rec, ok := t.records[id]; ok {
			rec.closeDone()
			delete(t.records, id)
		}
	}
}

// observePut strikes one arriving chunk off every open episode's pending set for
// that node. Only sync-sourced puts count: an inject is the operator putting the
// chunk there, not the network healing itself.
func (t *healTracker) observePut(node int, key string, source PutSource) {
	if source != PutSourceSync {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	for _, id := range t.order {
		rec, ok := t.records[id]
		if !ok || rec.settled {
			continue
		}
		set, ok := rec.pending[node]
		if !ok {
			continue
		}
		if _, ok := set[key]; !ok {
			continue
		}
		delete(set, key)
		rec.healed[node]++
		rec.healedAll++
		rec.lastPutAt = now
		if rec.healedAll >= rec.totalAll {
			// Nothing left to wait for: settle immediately rather than burning
			// a whole quiescence window.
			rec.settleLocked(now)
		}
	}
}

// settleLocked freezes the episode's span. Must be called under t.mu.
func (r *healRec) settleLocked(now time.Time) {
	if r.settled {
		return
	}
	r.settled = true
	last := r.lastPutAt
	if last.IsZero() {
		last = now
	}
	r.spanMs = last.Sub(r.t0).Milliseconds()
	r.closeDone()
}

// sweep settles episodes that have been quiet for settleAfter and returns every
// episode that has newly settled since the last sweep, including those that
// settled on their last arriving chunk.
func (t *healTracker) sweep() []Heal {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	var out []Heal
	for _, id := range t.order {
		rec, ok := t.records[id]
		if !ok {
			continue
		}
		if !rec.settled {
			// The quiescence clock runs from the episode start, not from the
			// first arrival: an episode where nothing at all heals must still
			// settle and report its residue rather than hang forever.
			last := rec.lastPutAt
			if last.IsZero() {
				last = rec.t0
			}
			if now.Sub(last) < t.settleAfter {
				continue
			}
			rec.settleLocked(now)
		}
		if rec.published {
			continue
		}
		rec.published = true
		out = append(out, rec.view())
	}
	return out
}

// stop closes the wait channel of every unsettled episode so callers blocked in
// WaitHeal wake up when the network shuts down. Idempotent.
func (t *healTracker) stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, rec := range t.records {
		rec.closeDone()
	}
}

// view renders the record as a Heal. Must be called under t.mu.
func (r *healRec) view() Heal {
	h := Heal{
		ID:         r.id,
		FromRadius: r.from,
		ToRadius:   r.to,
		Total:      r.totalAll,
		Healed:     r.healedAll,
		Remaining:  r.totalAll - r.healedAll,
		Settled:    r.settled,
		HealSpanMs: r.spanMs,
	}
	if !r.settled {
		last := r.lastPutAt
		if last.IsZero() {
			last = r.t0
		}
		h.HealSpanMs = last.Sub(r.t0).Milliseconds()
	}
	nodes := make([]int, 0, len(r.total))
	for node := range r.total {
		nodes = append(nodes, node)
	}
	sort.Ints(nodes)
	h.PerNode = make([]NodeHeal, 0, len(nodes))
	for _, node := range nodes {
		h.PerNode = append(h.PerNode, NodeHeal{Node: node, Total: r.total[node], Healed: r.healed[node]})
	}
	return h
}

// list returns the retained episodes, oldest first.
func (t *healTracker) list() []Heal {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Heal, 0, len(t.order))
	for _, id := range t.order {
		if rec, ok := t.records[id]; ok {
			out = append(out, rec.view())
		}
	}
	return out
}

// get returns a single episode by ID.
func (t *healTracker) get(id int) (Heal, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	rec, ok := t.records[id]
	if !ok {
		return Heal{}, false
	}
	return rec.view(), true
}

// wait returns a channel closed when the episode settles. Unknown episodes
// yield an already-closed channel so callers never block on one that finished
// before they asked.
func (t *healTracker) wait(id int) <-chan struct{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	if rec, ok := t.records[id]; ok {
		return rec.done
	}
	closed := make(chan struct{})
	close(closed)
	return closed
}
