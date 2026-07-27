// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sim

import (
	"sort"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// trackerRetain bounds how many batches are kept for reporting, so a
// long-running session does not grow without limit.
const trackerRetain = 32

// BatchMetrics are the derived timings for one injected batch. Percentiles are
// computed once, when the batch settles; a running batch reports them as zero.
type BatchMetrics struct {
	// SpanMs is the headline number: first inject to last put anywhere.
	SpanMs int64
	// InjectMs is how long the origin kept feeding chunks in (0 for a burst).
	InjectMs int64
	// TailMs is how long the batch kept spreading after feeding stopped.
	TailMs int64
	// Per-delivery timings are the distribution over *every* delivery of a
	// batch chunk: one sample per (chunk, receiving node) pair, measured from
	// that chunk's own origin put. Deliveries of chunks whose origin put was
	// never observed are excluded. This is a latency distribution, so p95
	// pulling away from p50 is evidence of queueing; a max-over-peers
	// statistic would instead grow with node count on its own.
	PerDeliveryP50Ms int64
	PerDeliveryP95Ms int64
	PerDeliveryMaxMs int64
}

// Batch is a point-in-time view of one tracked injection.
type Batch struct {
	ID           int
	Origin       int
	Chunks       int
	Replicas     int
	NodesReached int
	Settled      bool
	// LateReplicas counts replica puts that landed *after* the batch was
	// declared settled. Any non-zero value is direct proof the quiescence
	// window closed the batch too early and the span is truncated.
	LateReplicas int
	Metrics      BatchMetrics
}

// batchRec is the tracker's mutable record for a batch.
type batchRec struct {
	id     int
	origin int
	chunks int

	t0           time.Time
	lastInjectAt time.Time
	lastPutAt    time.Time

	replicas     int
	lateReplicas int
	nodes        map[int]struct{}

	// originPutAt is the origin-side put time per chunk, keyed by
	// addr.ByteString().
	originPutAt map[string]time.Time
	// latencies holds one sample per delivery (chunk x receiving node),
	// measured from that chunk's own origin put. It is dropped once the
	// percentiles are computed at settle time, so a settled batch retains only
	// its three summary numbers.
	latencies []int64

	settled bool
	metrics BatchMetrics
	done    chan struct{}
	// doneClosed guards close(done) so eviction, settling and shutdown can
	// never double-close it.
	doneClosed bool
}

// closeDone closes the batch's wait channel at most once. Must be called under
// the tracker mutex.
func (r *batchRec) closeDone() {
	if r.doneClosed {
		return
	}
	r.doneClosed = true
	close(r.done)
}

// tracker attributes reserve puts to the injection batch that produced them and
// declares a batch complete once no put for it has landed for settleAfter.
//
// It holds its own mutex and never calls back into the Network, so it is safe
// to read from Network.Snapshot while Network.mu is held.
type tracker struct {
	now         func() time.Time
	settleAfter time.Duration

	mu      sync.Mutex
	seq     int
	batches map[int]*batchRec
	order   []int          // batch IDs, oldest first
	byAddr  map[string]int // chunk address -> batch ID
}

func newTracker(settleAfter time.Duration, now func() time.Time) *tracker {
	return &tracker{
		now:         now,
		settleAfter: settleAfter,
		batches:     make(map[int]*batchRec),
		byAddr:      make(map[string]int),
	}
}

// register starts tracking a batch of addresses injected into origin and
// returns its ID.
func (t *tracker) register(origin int, addrs []swarm.Address) int {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.seq++
	rec := &batchRec{
		id:          t.seq,
		origin:      origin,
		chunks:      len(addrs),
		t0:          t.now(),
		nodes:       make(map[int]struct{}),
		originPutAt: make(map[string]time.Time, len(addrs)),
		done:        make(chan struct{}),
	}
	t.batches[rec.id] = rec
	t.order = append(t.order, rec.id)
	for _, a := range addrs {
		t.byAddr[a.ByteString()] = rec.id
	}
	t.evictLocked()
	return rec.id
}

// evictLocked drops the oldest batches beyond trackerRetain along with their
// address mappings. Must be called under t.mu.
func (t *tracker) evictLocked() {
	for len(t.order) > trackerRetain {
		id := t.order[0]
		t.order = t.order[1:]
		if rec, ok := t.batches[id]; ok {
			rec.closeDone()
			delete(t.batches, id)
		}
		for addr, owner := range t.byAddr {
			if owner == id {
				delete(t.byAddr, addr)
			}
		}
	}
}

// observePut folds one reserve put into its batch. Puts for addresses that
// belong to no tracked batch are ignored; that is the common case, since
// live-sync traffic carries chunks from long-evicted batches.
func (t *tracker) observePut(node int, addr swarm.Address, source PutSource) {
	key := addr.ByteString()

	t.mu.Lock()
	defer t.mu.Unlock()

	id, ok := t.byAddr[key]
	if !ok {
		return
	}
	rec, ok := t.batches[id]
	if !ok {
		return
	}
	if rec.settled {
		// The batch is closed, but a replica still arrived. Count it: a
		// non-zero LateReplicas is self-evidencing proof that the quiescence
		// window truncated the measurement, without needing an externally
		// computed expected replica count to compare against.
		if source == PutSourceSync {
			rec.lateReplicas++
		}
		return
	}

	now := t.now()
	// Both sources advance lastPutAt: during a dripped inject the origin puts
	// are activity too, and must not let the batch settle mid-drip.
	rec.lastPutAt = now

	switch source {
	case PutSourceInject:
		if _, seen := rec.originPutAt[key]; !seen {
			rec.originPutAt[key] = now
		}
		rec.lastInjectAt = now
	case PutSourceSync:
		rec.replicas++
		rec.nodes[node] = struct{}{}
		// Every delivery is its own latency sample, so the percentiles
		// describe the delivery distribution rather than a max over peers.
		if origin, seen := rec.originPutAt[key]; seen {
			rec.latencies = append(rec.latencies, now.Sub(origin).Milliseconds())
		}
	}
}

// sweep settles every batch that has been quiet for settleAfter and returns
// those newly settled.
func (t *tracker) sweep() []Batch {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	var out []Batch
	for _, id := range t.order {
		rec, ok := t.batches[id]
		if !ok || rec.settled {
			continue
		}
		// The quiescence clock only starts once the first put lands. A batch
		// with no put yet (a drip slower than settleAfter, or a chunk still in
		// flight) must not settle: doing so would report SpanMs=0/Replicas=0
		// and then discard every subsequent put. The bench per-cell timeout
		// and the bounded retention are the backstop for a batch that never
		// receives a put at all.
		if rec.lastPutAt.IsZero() {
			continue
		}
		if now.Sub(rec.lastPutAt) < t.settleAfter {
			continue
		}
		rec.settled = true
		rec.metrics = rec.computeMetrics()
		// The raw per-delivery samples are only needed to derive the
		// percentiles; drop them so a settled batch retains O(1) memory.
		rec.latencies = nil
		rec.closeDone()
		out = append(out, rec.view())
	}
	return out
}

// stop closes the wait channel of every batch that has not settled, so callers
// blocked in WaitBatch wake up when the network shuts down. Idempotent.
func (t *tracker) stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, rec := range t.batches {
		rec.closeDone()
	}
}

// computeMetrics derives the batch timings. Must be called under t.mu.
func (r *batchRec) computeMetrics() BatchMetrics {
	last := r.lastPutAt
	if last.IsZero() {
		last = r.t0
	}
	m := BatchMetrics{SpanMs: last.Sub(r.t0).Milliseconds()}
	if !r.lastInjectAt.IsZero() {
		m.InjectMs = r.lastInjectAt.Sub(r.t0).Milliseconds()
	}
	m.TailMs = m.SpanMs - m.InjectMs

	// One sample per delivery. Chunks that never reached a peer contribute no
	// samples at all; counting them as zero would hide the real tail.
	if len(r.latencies) == 0 {
		return m
	}
	lat := append([]int64(nil), r.latencies...)
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	m.PerDeliveryP50Ms = percentile(lat, 50)
	m.PerDeliveryP95Ms = percentile(lat, 95)
	m.PerDeliveryMaxMs = lat[len(lat)-1]
	return m
}

// percentile returns the p-th percentile of a sorted slice using
// nearest-rank, so the result is always an observed value.
func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := (p*len(sorted) + 99) / 100 // ceil(p/100 * n)
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

// view renders the record as a Batch. Must be called under t.mu. A running
// batch gets live span/inject/tail and zero percentiles, so that reporting a
// large in-flight batch every snapshot stays cheap.
func (r *batchRec) view() Batch {
	b := Batch{
		ID:           r.id,
		Origin:       r.origin,
		Chunks:       r.chunks,
		Replicas:     r.replicas,
		NodesReached: len(r.nodes),
		Settled:      r.settled,
		LateReplicas: r.lateReplicas,
	}
	if r.settled {
		b.Metrics = r.metrics
		return b
	}
	last := r.lastPutAt
	if last.IsZero() {
		last = r.t0
	}
	b.Metrics.SpanMs = last.Sub(r.t0).Milliseconds()
	if !r.lastInjectAt.IsZero() {
		b.Metrics.InjectMs = r.lastInjectAt.Sub(r.t0).Milliseconds()
	}
	b.Metrics.TailMs = b.Metrics.SpanMs - b.Metrics.InjectMs
	return b
}

// list returns the retained batches, oldest first.
func (t *tracker) list() []Batch {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Batch, 0, len(t.order))
	for _, id := range t.order {
		if rec, ok := t.batches[id]; ok {
			out = append(out, rec.view())
		}
	}
	return out
}

// get returns a single batch by ID.
func (t *tracker) get(id int) (Batch, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	rec, ok := t.batches[id]
	if !ok {
		return Batch{}, false
	}
	return rec.view(), true
}

// wait returns a channel closed when the batch settles. Unknown or already
// settled batches yield an already-closed channel so callers never block on a
// batch that finished before they asked.
func (t *tracker) wait(id int) <-chan struct{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	if rec, ok := t.batches[id]; ok {
		return rec.done
	}
	closed := make(chan struct{})
	close(closed)
	return closed
}
