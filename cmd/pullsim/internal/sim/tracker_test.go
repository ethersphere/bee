// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sim

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// fakeClock is a manually advanced clock for deterministic tracker tests.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func addrN(b byte) swarm.Address {
	buf := make([]byte, swarm.HashSize)
	buf[0] = b
	return swarm.NewAddress(buf)
}

func newTestTracker(settle time.Duration) (*tracker, *fakeClock) {
	c := &fakeClock{t: time.Unix(0, 0)}
	return newTracker(settle, c.now), c
}

func TestTrackerSpanEndsAtLastPut(t *testing.T) {
	tr, c := newTestTracker(3 * time.Second)
	a := addrN(1)

	id := tr.register(0, []swarm.Address{a})
	tr.observePut(0, a, PutSourceInject)

	c.advance(400 * time.Millisecond)
	tr.observePut(1, a, PutSourceSync)

	// Quiet period elapses; it must not be counted in the span.
	c.advance(3 * time.Second)
	settled := tr.sweep()

	if len(settled) != 1 {
		t.Fatalf("got %d settled batches, want 1", len(settled))
	}
	b := settled[0]
	if b.ID != id {
		t.Errorf("got batch ID %d, want %d", b.ID, id)
	}
	if !b.Settled {
		t.Error("batch not marked settled")
	}
	if b.Metrics.SpanMs != 400 {
		t.Errorf("got SpanMs %d, want 400", b.Metrics.SpanMs)
	}
	if b.Metrics.TailMs != 400 {
		t.Errorf("got TailMs %d, want 400", b.Metrics.TailMs)
	}
	if b.Replicas != 1 {
		t.Errorf("got Replicas %d, want 1", b.Replicas)
	}
	if b.NodesReached != 1 {
		t.Errorf("got NodesReached %d, want 1", b.NodesReached)
	}
}

func TestTrackerOriginPutsDeferSettling(t *testing.T) {
	tr, c := newTestTracker(3 * time.Second)
	a, bAddr := addrN(1), addrN(2)
	tr.register(0, []swarm.Address{a, bAddr})

	tr.observePut(0, a, PutSourceInject)
	c.advance(2 * time.Second)
	if got := tr.sweep(); len(got) != 0 {
		t.Fatalf("settled early: %d", len(got))
	}
	// A second origin put (drip) must reset the quiescence window.
	tr.observePut(0, bAddr, PutSourceInject)
	c.advance(2 * time.Second)
	if got := tr.sweep(); len(got) != 0 {
		t.Fatalf("settled despite recent origin put: %d", len(got))
	}
	c.advance(1 * time.Second)
	settled := tr.sweep()
	if len(settled) != 1 {
		t.Fatalf("got %d settled, want 1", len(settled))
	}
	if settled[0].Metrics.InjectMs != 2000 {
		t.Errorf("got InjectMs %d, want 2000", settled[0].Metrics.InjectMs)
	}
	if settled[0].Metrics.TailMs != 0 {
		t.Errorf("got TailMs %d, want 0", settled[0].Metrics.TailMs)
	}
}

func TestTrackerCountsDistinctNodesOnly(t *testing.T) {
	tr, c := newTestTracker(time.Second)
	a, bAddr := addrN(1), addrN(2)
	tr.register(0, []swarm.Address{a, bAddr})
	tr.observePut(0, a, PutSourceInject)
	tr.observePut(0, bAddr, PutSourceInject)

	c.advance(100 * time.Millisecond)
	tr.observePut(1, a, PutSourceSync)
	tr.observePut(1, bAddr, PutSourceSync) // same node, second chunk
	tr.observePut(2, a, PutSourceSync)

	c.advance(time.Second)
	settled := tr.sweep()
	if len(settled) != 1 {
		t.Fatalf("got %d settled, want 1", len(settled))
	}
	if settled[0].Replicas != 3 {
		t.Errorf("got Replicas %d, want 3", settled[0].Replicas)
	}
	if settled[0].NodesReached != 2 {
		t.Errorf("got NodesReached %d, want 2", settled[0].NodesReached)
	}
}

func TestTrackerPercentilesSkipUnreplicatedChunks(t *testing.T) {
	tr, c := newTestTracker(time.Second)
	a, bAddr := addrN(1), addrN(2)
	tr.register(0, []swarm.Address{a, bAddr})
	tr.observePut(0, a, PutSourceInject)
	tr.observePut(0, bAddr, PutSourceInject)

	// Only chunk a is ever replicated, at +500ms.
	c.advance(500 * time.Millisecond)
	tr.observePut(1, a, PutSourceSync)

	c.advance(time.Second)
	settled := tr.sweep()
	m := settled[0].Metrics
	if m.PerDeliveryMaxMs != 500 {
		t.Errorf("got PerDeliveryMaxMs %d, want 500", m.PerDeliveryMaxMs)
	}
	// The never-replicated chunk must not pull the median to 0.
	if m.PerDeliveryP50Ms != 500 {
		t.Errorf("got PerDeliveryP50Ms %d, want 500", m.PerDeliveryP50Ms)
	}
}

func TestTrackerIgnoresUnknownAddresses(t *testing.T) {
	tr, c := newTestTracker(time.Second)
	a := addrN(1)
	tr.register(0, []swarm.Address{a})
	tr.observePut(0, a, PutSourceInject)

	tr.observePut(3, addrN(99), PutSourceSync) // not part of any batch

	c.advance(time.Second)
	settled := tr.sweep()
	if len(settled) != 1 {
		t.Fatalf("got %d settled, want 1", len(settled))
	}
	if settled[0].Replicas != 0 {
		t.Errorf("got Replicas %d, want 0", settled[0].Replicas)
	}
}

func TestTrackerSweepIsIdempotent(t *testing.T) {
	tr, c := newTestTracker(time.Second)
	a := addrN(1)
	tr.register(0, []swarm.Address{a})
	tr.observePut(0, a, PutSourceInject)
	c.advance(time.Second)

	if got := tr.sweep(); len(got) != 1 {
		t.Fatalf("first sweep got %d, want 1", len(got))
	}
	if got := tr.sweep(); len(got) != 0 {
		t.Fatalf("second sweep got %d, want 0", len(got))
	}
}

func TestTrackerWaitClosesOnSettle(t *testing.T) {
	tr, c := newTestTracker(time.Second)
	a := addrN(1)
	id := tr.register(0, []swarm.Address{a})
	tr.observePut(0, a, PutSourceInject)

	ch := tr.wait(id)
	select {
	case <-ch:
		t.Fatal("wait channel closed before settle")
	default:
	}

	c.advance(time.Second)
	tr.sweep()

	select {
	case <-ch:
	default:
		t.Fatal("wait channel not closed after settle")
	}

	// A wait on an already-settled batch must not block.
	select {
	case <-tr.wait(id):
	default:
		t.Fatal("wait on settled batch did not return a closed channel")
	}
	// A wait on an unknown batch must not block either.
	select {
	case <-tr.wait(99999):
	default:
		t.Fatal("wait on unknown batch did not return a closed channel")
	}
}

func TestTrackerRetainsBoundedHistory(t *testing.T) {
	tr, c := newTestTracker(time.Millisecond)
	for i := 0; i < trackerRetain+5; i++ {
		a := addrN(byte(i))
		tr.register(0, []swarm.Address{a})
		tr.observePut(0, a, PutSourceInject)
		c.advance(time.Second)
		tr.sweep()
	}
	if got := len(tr.list()); got != trackerRetain {
		t.Errorf("got %d retained batches, want %d", got, trackerRetain)
	}
	// Evicted batches must not leak address mappings.
	tr.mu.Lock()
	n := len(tr.byAddr)
	tr.mu.Unlock()
	if n != trackerRetain {
		t.Errorf("got %d address mappings, want %d", n, trackerRetain)
	}
}

func TestTrackerListIsNewestLast(t *testing.T) {
	tr, _ := newTestTracker(time.Minute)
	id1 := tr.register(0, []swarm.Address{addrN(1)})
	id2 := tr.register(1, []swarm.Address{addrN(2)})
	got := tr.list()
	if len(got) != 2 {
		t.Fatalf("got %d batches, want 2", len(got))
	}
	if got[0].ID != id1 || got[1].ID != id2 {
		t.Errorf("got order [%d %d], want [%d %d]", got[0].ID, got[1].ID, id1, id2)
	}
	if got[1].Origin != 1 {
		t.Errorf("got Origin %d, want 1", got[1].Origin)
	}
}

// I4: a batch whose first chunk has not landed yet must not settle. Before the
// fix, `last = t0` fallback let a drip slower than the settle window settle
// with SpanMs=0/Replicas=0, after which every real put was discarded.
func TestTrackerDoesNotSettleBeforeFirstPut(t *testing.T) {
	tr, c := newTestTracker(3 * time.Second)
	a := addrN(1)
	id := tr.register(0, []swarm.Address{a})

	// Far past the settle window, but nothing has been put yet.
	c.advance(30 * time.Second)
	if got := tr.sweep(); len(got) != 0 {
		t.Fatalf("settled with no puts: %+v", got)
	}
	if b, _ := tr.get(id); b.Settled {
		t.Fatal("batch reported settled before its first put")
	}

	// The first chunk finally drips in; the clock starts here.
	tr.observePut(0, a, PutSourceInject)
	c.advance(500 * time.Millisecond)
	tr.observePut(1, a, PutSourceSync)

	c.advance(2 * time.Second)
	if got := tr.sweep(); len(got) != 0 {
		t.Fatalf("settled before the window elapsed: %+v", got)
	}
	c.advance(1 * time.Second)
	settled := tr.sweep()
	if len(settled) != 1 {
		t.Fatalf("got %d settled, want 1", len(settled))
	}
	// The span must start at the *inject*, not at the late first put: t0 is
	// still the register time, so 30s + 500ms.
	if want := int64(30500); settled[0].Metrics.SpanMs != want {
		t.Errorf("got SpanMs %d, want %d", settled[0].Metrics.SpanMs, want)
	}
	if settled[0].Replicas != 1 {
		t.Errorf("got Replicas %d, want 1", settled[0].Replicas)
	}
}

// I5: Network.Close must wake anything blocked on an unsettled batch.
func TestTrackerStopWakesUnsettledBatches(t *testing.T) {
	tr, _ := newTestTracker(time.Hour)
	a := addrN(1)
	id := tr.register(0, []swarm.Address{a})
	tr.observePut(0, a, PutSourceInject)

	ch := tr.wait(id)
	select {
	case <-ch:
		t.Fatal("wait channel closed before stop")
	default:
	}

	tr.stop()
	select {
	case <-ch:
	default:
		t.Fatal("wait channel not closed by stop")
	}
	if b, _ := tr.get(id); b.Settled {
		t.Error("stop must not mark a batch settled")
	}
	// Double Close, and a stop after a settle, must not double-close.
	tr.stop()
	tr.stop()
}

func TestTrackerStopAfterSettleDoesNotDoubleClose(t *testing.T) {
	tr, c := newTestTracker(time.Second)
	a := addrN(1)
	tr.register(0, []swarm.Address{a})
	tr.observePut(0, a, PutSourceInject)
	c.advance(time.Second)
	if got := tr.sweep(); len(got) != 1 {
		t.Fatalf("got %d settled, want 1", len(got))
	}
	tr.stop() // must be a no-op for the already-closed channel
}

// I1: replicas arriving after the settle window are counted, which is direct
// evidence the window truncated the measurement.
func TestTrackerCountsLateReplicas(t *testing.T) {
	tr, c := newTestTracker(time.Second)
	a := addrN(1)
	id := tr.register(0, []swarm.Address{a})
	tr.observePut(0, a, PutSourceInject)
	c.advance(100 * time.Millisecond)
	tr.observePut(1, a, PutSourceSync)

	c.advance(time.Second)
	settled := tr.sweep()
	if len(settled) != 1 {
		t.Fatalf("got %d settled, want 1", len(settled))
	}
	if settled[0].LateReplicas != 0 {
		t.Fatalf("got LateReplicas %d at settle time, want 0", settled[0].LateReplicas)
	}

	// A slow hop lands after the batch was closed.
	c.advance(5 * time.Second)
	tr.observePut(2, a, PutSourceSync)
	tr.observePut(3, a, PutSourceSync)
	// Origin-side puts after settling are not replicas and must not count.
	tr.observePut(0, a, PutSourceInject)
	// Nor must puts for addresses belonging to no tracked batch.
	tr.observePut(4, addrN(99), PutSourceSync)

	b, ok := tr.get(id)
	if !ok {
		t.Fatal("batch not retained")
	}
	if b.LateReplicas != 2 {
		t.Errorf("got LateReplicas %d, want 2", b.LateReplicas)
	}
	// The settled metrics must not be rewritten by the late arrivals.
	if b.Metrics.SpanMs != 100 {
		t.Errorf("got SpanMs %d, want 100 (late puts must not extend the span)", b.Metrics.SpanMs)
	}
	if b.Replicas != 1 {
		t.Errorf("got Replicas %d, want 1", b.Replicas)
	}
}

// I2: percentiles are over every delivery, not over each chunk's last replica.
// A max-over-peers statistic collapses p50 onto p95; a delivery distribution
// keeps them apart, which is the whole point of reporting both.
func TestTrackerPerDeliveryPercentilesSpread(t *testing.T) {
	tr, c := newTestTracker(time.Second)
	a := addrN(1)
	tr.register(0, []swarm.Address{a})
	tr.observePut(0, a, PutSourceInject)

	// One chunk delivered to 20 nodes, spread from +100ms to +2000ms.
	for i := 1; i <= 20; i++ {
		c.advance(100 * time.Millisecond)
		tr.observePut(i, a, PutSourceSync)
	}

	c.advance(time.Second)
	settled := tr.sweep()
	if len(settled) != 1 {
		t.Fatalf("got %d settled, want 1", len(settled))
	}
	m := settled[0].Metrics
	// Nearest-rank over [100..2000]: p50 = 1000, p95 = 1900, max = 2000.
	if m.PerDeliveryP50Ms != 1000 {
		t.Errorf("got PerDeliveryP50Ms %d, want 1000", m.PerDeliveryP50Ms)
	}
	if m.PerDeliveryP95Ms != 1900 {
		t.Errorf("got PerDeliveryP95Ms %d, want 1900", m.PerDeliveryP95Ms)
	}
	if m.PerDeliveryMaxMs != 2000 {
		t.Errorf("got PerDeliveryMaxMs %d, want 2000", m.PerDeliveryMaxMs)
	}
	// The statistic must actually distinguish the two: a per-chunk
	// last-replica statistic would report p50 == p95 == max here.
	if m.PerDeliveryP95Ms <= m.PerDeliveryP50Ms {
		t.Errorf("p95 %d did not pull away from p50 %d", m.PerDeliveryP95Ms, m.PerDeliveryP50Ms)
	}
}

// The raw samples are bounded by chunks x nodes per batch, so they must not
// outlive the settle that summarizes them.
func TestTrackerDropsSamplesOnSettle(t *testing.T) {
	tr, c := newTestTracker(time.Second)
	a := addrN(1)
	id := tr.register(0, []swarm.Address{a})
	tr.observePut(0, a, PutSourceInject)
	for i := 1; i <= 5; i++ {
		c.advance(10 * time.Millisecond)
		tr.observePut(i, a, PutSourceSync)
	}
	c.advance(time.Second)
	tr.sweep()

	tr.mu.Lock()
	n := len(tr.batches[id].latencies)
	tr.mu.Unlock()
	if n != 0 {
		t.Errorf("settled batch still retains %d raw latency samples", n)
	}
}
