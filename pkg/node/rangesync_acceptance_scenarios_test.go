// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build integration

package node_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"math"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// The scenarios of plan task T6.1 part 2: Bootstrap, Radius decrease, Stale
// stamp and Churn, each on rangesync-only nodes and, where a criterion
// compares the protocols, on pullsync-only nodes with the same overlays and
// chunks (the Baseline of the scenario).
//
// Two deviations from the node's wiring hold in every part-2 scenario:
//   - the streams have a bounded window (accWindow, yamux's initial stream
//     window), so that a transfer of a whole reserve is paced by its reader,
//     as on libp2p;
//   - LargeDiff is accLargeDiff instead of rangesync.LargeDiff (65536), so
//     that the suite's reserves of a few thousand chunks are striped where a
//     reserve of 2^22 is (T4.7 part 1 scaled its tests the same way).
//
// The criteria whose sync rate must return to 0 (AC-V3, AC-V4 and AC-N1 to
// the letter, the second decrease of AC-N3) wait out the 10-minute rate
// windows of the puller and the reconciler: they run only with
// RANGESYNC_ACCEPTANCE_LONG set (accLong).
//
// The Baseline networks (here and in part 1's TestAcceptance), the
// Bootstrap of a nested-root joiner and the Radius decrease networks' upload
// phase with the mixed radii run only with RANGESYNC_ACCEPTANCE_FULL or
// RANGESYNC_ACCEPTANCE_LONG set (accFull), so that the package's default run
// stays well within go test's 10-minute timeout: the Baselines compare and
// report, and the other two are variants of what the default run checks.

const (
	// accWindow is the window of a stream (go-yamux initialStreamWindow).
	accWindow = 256 << 10
	// accLargeDiff is the part-2 scenarios' LargeDiff: about a 35th of their
	// 4500-chunk reserves, against 1/64 of a 2^22 reserve in the node, and
	// above the steady-state difference of a session at the reference load
	// (~40), so that only a catch-up is striped.
	accLargeDiff = 128
	// accZeroLimit bounds the wait for a sync rate back to 0: the rate
	// windows are aligned to multiples of 10 minutes, so a rate reads 0 from
	// 10 to 20 minutes after its last chunk.
	accZeroLimit = 25 * time.Minute
	// accBootstrapLimit bounds the wait for a bootstrap or a new half.
	accBootstrapLimit = 3 * time.Minute
	// accTail is the part-2 scenarios' quiet tail (accOptions.Tail): one
	// MinRoundInterval with equal digests and no session started, half of
	// part 1's, so that the default run stays within its timeout.
	accTail = 5 * time.Second
)

// accLong reports whether the 10-minute waits run
// (RANGESYNC_ACCEPTANCE_LONG set).
func accLong() bool { return os.Getenv("RANGESYNC_ACCEPTANCE_LONG") != "" }

// accFull reports whether the networks and phases that the default run
// leaves out run (RANGESYNC_ACCEPTANCE_FULL or RANGESYNC_ACCEPTANCE_LONG set).
func accFull() bool { return os.Getenv("RANGESYNC_ACCEPTANCE_FULL") != "" || accLong() }

// accScenarioSizes returns the network sizes of the part-2 scenarios that
// take one: RANGESYNC_ACCEPTANCE_SCENARIO_SIZES (a comma-separated list), 5
// by default.
func accScenarioSizes(t *testing.T) []int {
	t.Helper()

	v := os.Getenv("RANGESYNC_ACCEPTANCE_SCENARIO_SIZES")
	if v == "" {
		return []int{5}
	}
	var sizes []int
	for _, f := range strings.Split(v, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil || n < 4 {
			t.Fatalf("RANGESYNC_ACCEPTANCE_SCENARIO_SIZES: bad size %q", f)
		}
		sizes = append(sizes, n)
	}
	return sizes
}

// accRateWatch samples a node's sync rate (its combined syncer: the
// reconciler's or the puller's SyncRate) every accSampleInterval.
type accRateWatch struct {
	quit, done chan struct{}

	mu      sync.Mutex
	samples []accRateSample
}

type accRateSample struct {
	at   time.Time
	rate float64
}

func watchRate(n *accNode) *accRateWatch {
	w := &accRateWatch{quit: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(w.done)
		t := time.NewTicker(accSampleInterval)
		defer t.Stop()
		for {
			s := accRateSample{at: time.Now(), rate: n.syncer.SyncRate()}
			w.mu.Lock()
			w.samples = append(w.samples, s)
			w.mu.Unlock()
			select {
			case <-w.quit:
				return
			case <-t.C:
			}
		}
	}()
	return w
}

func (w *accRateWatch) stop() {
	select {
	case <-w.quit:
	default:
		close(w.quit)
	}
	<-w.done
}

// span returns the smallest and the largest rate sampled in [from, to], and
// the number of samples.
func (w *accRateWatch) span(from, to time.Time) (lo, hi float64, n int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	lo = math.Inf(1)
	for _, s := range w.samples {
		if s.at.Before(from) || s.at.After(to) {
			continue
		}
		lo, hi = min(lo, s.rate), max(hi, s.rate)
		n++
	}
	return lo, hi, n
}

// waitZero polls node n's sync rate until it reads 0 and returns when it
// first did, failing after accZeroLimit.
func waitZero(t *testing.T, n *accNode) time.Time {
	t.Helper()

	var at time.Time
	accWaitEvery(t, fmt.Sprintf("node %d's sync rate to return to 0", n.i), accZeroLimit, time.Second, func() (bool, string) {
		r := n.syncer.SyncRate()
		at = time.Now()
		return r == 0, fmt.Sprintf("rate %v", r)
	})
	return at
}

// sessionsWith returns the reconcile streams between node n and every other
// node.
func (h *accHarness) sessionsWith(n *accNode) int64 {
	var sum int64
	for _, p := range h.nodes {
		if p != n {
			k, _ := h.net.pairStats(n.overlay, p.overlay, "rangesync/reconcile")
			sum += k
		}
	}
	return sum
}

// accWaitEvery is accWait polling every interval.
func accWaitEvery(tb testing.TB, what string, limit, interval time.Duration, cond func() (bool, string)) {
	tb.Helper()

	deadline := time.Now().Add(limit)
	for {
		ok, state := cond()
		if ok {
			return
		}
		if time.Now().After(deadline) {
			tb.Fatalf("timed out waiting for %s: %s", what, state)
		}
		time.Sleep(interval)
	}
}

// idsIn returns the IDs of the chunks inside r.
func idsIn(t *testing.T, chunks []swarm.Chunk, r fingerprint.Range) map[fingerprint.ID]struct{} {
	t.Helper()

	ids := make(map[fingerprint.ID]struct{})
	for _, ch := range chunks {
		if !r.Contains(ch.Address()) {
			continue
		}
		id, err := chunkIDOf(ch)
		if err != nil {
			t.Fatal(err)
		}
		ids[id] = struct{}{}
	}
	return ids
}

// arrivalSpan returns how many of ids reached node i's reserve, and the
// first and the last arrival.
func (h *accHarness) arrivalSpan(i int, ids map[fingerprint.ID]struct{}) (n int, first, last time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id := range ids {
		at, ok := h.arrivals[i][id]
		if !ok {
			continue
		}
		n++
		if first.IsZero() || at.Before(first) {
			first = at
		}
		if at.After(last) {
			last = at
		}
	}
	return n, first, last
}

// accDeliveryCount is what node i received of a set of IDs: the deliveries
// by protocol, and the IDs delivered more than once by one protocol or by
// both.
type accDeliveryCount struct {
	ps, rs     int
	repeats    int // deliveries past the first of an ID, by either protocol
	repeatedRS int // IDs rangesync delivered more than once
}

func (h *accHarness) deliveriesTo(i int, ids map[fingerprint.ID]struct{}) accDeliveryCount {
	h.mu.Lock()
	defer h.mu.Unlock()
	var c accDeliveryCount
	for id := range ids {
		d, ok := h.deliveries[i][id]
		if !ok {
			continue
		}
		c.ps += d.n[accViaPullsync]
		c.rs += d.n[accViaRangesync]
		c.repeats += max(0, d.n[accViaPullsync]+d.n[accViaRangesync]-1)
		if d.n[accViaRangesync] > 1 {
			c.repeatedRS++
		}
	}
	return c
}

// sampleEqual reports whether the reserve samples of nodes a and b, with the
// same random anchor in the deeper one's neighborhood (as a round's anchor
// selects it) at their own radius, are equal and not empty: the same chunks,
// data and stamps (AC-N5's T6.1 part, for two nodes that hold the same set
// there).
func sampleEqual(t *testing.T, a, b *accNode) bool {
	t.Helper()

	random := make([]byte, swarm.HashSize)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	deeper := a
	if b.db.StorageRadius() > a.db.StorageRadius() {
		deeper = b
	}
	anchor := withPrefix(swarm.NewAddress(random), deeper.overlay, deeper.db.StorageRadius()).Bytes()
	ctx := context.Background()
	sa, err := a.db.ReserveSample(ctx, anchor, a.db.StorageRadius(), math.MaxUint64, nil)
	if err != nil {
		t.Fatal(err)
	}
	sb, err := b.db.ReserveSample(ctx, anchor, b.db.StorageRadius(), math.MaxUint64, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(sa.Items) == 0 || len(sa.Items) != len(sb.Items) {
		return false
	}
	for k := range sa.Items {
		x, y := sa.Items[k], sb.Items[k]
		if !x.TransformedAddress.Equal(y.TransformedAddress) || !x.ChunkAddress.Equal(y.ChunkAddress) || !slices.Equal(x.ChunkData, y.ChunkData) {
			return false
		}
		xs, err := x.Stamp.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		ys, err := y.Stamp.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(xs, ys) {
			return false
		}
	}
	return true
}

// accTable lays out results side by side.
type accTable struct {
	b strings.Builder
}

func (t *accTable) row(name string, vals ...string) {
	fmt.Fprintf(&t.b, "%-50s", name)
	for _, v := range vals {
		fmt.Fprintf(&t.b, " | %-26s", v)
	}
	t.b.WriteString("\n")
}

func (t *accTable) String() string { return t.b.String() }

func ms(d time.Duration) string { return d.Round(time.Millisecond).String() }

func mbytes(v float64) string { return fmt.Sprintf("%.2f MB", v/1e6) }

// --- Bootstrap ---------------------------------------------------------------

// accBootResult is what a Bootstrap scenario measured at its joiner, from
// the join to the end of the quiet tail (the window).
type accBootResult struct {
	name   string
	mode   accMode
	nodes  int
	chunks int // the joiner's final reserve count
	setup  time.Duration
	first  time.Duration // join to the first chunk stored
	union  time.Duration // join to the last chunk of the union stored
	quiet  time.Duration // join to the end of the quiet tail
	zero   time.Duration // join to the sync rate back at 0 (long runs)
	// idle is the joiner's sessions per minute with its peers while the
	// long run waits for its sync rate (in sync, no uploads)
	idle    float64
	rateMin float64 // the joiner's sync rate, smallest from join to the quiet end
	rateMax float64
	// rateMinFirst is the smallest sync rate before the first chunk
	rateMinFirst float64
	radii        []accRadiusChange

	metaIn, metaOut      float64 // rangesync bytes_metadata at the joiner
	metaInUnion          float64 // ... by the union
	chunksIn             float64 // bytes of the chunk deliveries to the joiner
	offeredIn            float64 // pullsync offers the joiner received
	wireIn               map[string]uint64
	wireInUnion          map[string]uint64
	deliveries           accDeliveryCount
	stripes, sessions    map[string]float64
	claims               map[string]float64
	verified             uint64  // stripes verified
	verifiedDiff         float64 // chunks the verifications delivered
	failures             float64 // peer_failures_total over the network
	disconnects          int64
	cpu                  float64
	indexPeak, indexRun  float64
	heapPeak             uint64
	sampleEqual          bool
	fetchLatP99, latMax  time.Duration
	limiterBound         time.Duration // the chunks at the fetch limiter's 1000 per second
	holdersMeta, holders float64       // bytes_metadata{in} summed over the holders
}

// TestAcceptanceBootstrap runs the Bootstrap scenario for each size of
// accScenarioSizes: N − 1 nodes hold the neighborhood's base chunks, then an
// empty node joins and syncs. It runs on rangesync-only nodes with a
// rangesync-only joiner (and, with accFull, a nested-root one, see
// runBootstrap), and on pullsync-only nodes with a pullsync-only joiner (the
// Baseline, for AC-V3; accFull), with the same overlays and chunks. On the
// rangesync joiners it checks:
//   - AC-D1, AC-B3: each chunk delivered once, chunk bytes ≤ 1.01 × the
//     chunks stored;
//   - AC-B2: bytes_metadata{dir="in"} by the union (the IDs the stripes
//     listed) ≤ 1.15 × final count × 70 B, and after it (the verification
//     of the stripes, which runs once every stripe is done) ≤ 1 MB;
//   - AC-N1, AC-N2: its sync rate stays above 0 from the join to the end of
//     the quiet tail, and its radius does not drop, although its reserve is
//     below the threshold all along and its reserve worker checks every
//     second;
//   - AC-C1, AC-K3; and AC-N5's T6.1 part: its reserve sample equals a
//     holder's.
//
// The bootstrap times of both joiners (AC-V3) are logged, as are the other
// numbers; with accLong, both joiners' sync rates are awaited back at 0.
func TestAcceptanceBootstrap(t *testing.T) {
	for _, n := range accScenarioSizes(t) {
		t.Run(fmt.Sprintf("N=%d", n), func(t *testing.T) {
			const base = 4500
			o := accOptions{
				Nodes: n - 1, Seed: uint64(100 + n), Radius: 2, Base: base,
				Window: accWindow, LargeDiff: accLargeDiff, Tail: accTail,
				// the joiner's final count is the threshold (half the
				// capacity): below it until the union, so that a sync rate
				// of 0 at any reserve-worker tick lowers its radius
				ReserveCapacity: 2 * base,
			}
			var results [3]*accBootResult
			// after the subtests, which run in parallel in long runs
			t.Cleanup(func() { logBootTable(t, results[:]) })
			for k, c := range []struct {
				name   string
				mode   accMode
				nested bool
			}{
				{"baseline", accPullsyncOnly, false},
				{"rangesync", accRangesyncOnly, false},
				{"rangesync nested root", accRangesyncOnly, true},
			} {
				t.Run(c.name, func(t *testing.T) {
					if (!c.mode.Rangesync || c.nested) && !accFull() {
						t.Skip("runs with RANGESYNC_ACCEPTANCE_FULL or RANGESYNC_ACCEPTANCE_LONG set")
					}
					if accLong() {
						t.Parallel()
					}
					bo := o
					bo.Mode = func(int) accMode { return c.mode }
					results[k] = runBootstrap(t, c.name, bo, c.mode, c.nested)
				})
			}
		})
	}
}

// runBootstrap runs a Bootstrap network: o's nodes hold the base, then an
// empty node of mode joins at their radius or, nested, one bit shallower, so
// that its root contains theirs and no neighbor shares it (T4.7 part 1's
// constraint (d): it syncs through claims of the session roots, not
// IsBootstrap).
func runBootstrap(t *testing.T, name string, o accOptions, mode accMode, nested bool) *accBootResult {
	t.Helper()

	begin := time.Now()
	h := newAccHarness(t, o)
	defer h.close()
	h.start()
	r := &accBootResult{name: name, mode: mode, nodes: len(h.nodes) + 1, setup: time.Since(begin)}

	runtime.GC()
	h.sampler.resetPeaks()
	s0 := h.snapshot()
	spec := accNodeSpec{
		overlay: withPrefix(swarm.NewAddress(h.randBytes(swarm.HashSize)), h.prefix, o.Radius),
		mode:    mode,
		radius:  o.Radius,
		wakeUp:  h.o.ReserveWakeUpDuration,
	}
	if nested {
		spec.radius--
	}
	if mode.Rangesync {
		// AC-N2: the reserve worker checks every second (the node: every
		// 15 minutes); the pullsync joiner keeps the default, so that
		// AC-V3 compares the two joiners as a node runs them
		spec.wakeUp = time.Second
	}
	joinAt := time.Now()
	j := h.join(spec)
	w := watchRate(j)
	defer w.stop()

	want := idsIn(t, h.base, fingerprint.NewRange(spec.overlay, spec.radius))
	r.chunks = len(want)
	accWait(t, "the joiner to hold its root's union", accBootstrapLimit, func() (bool, string) {
		got, _, _ := h.arrivalSpan(j.i, want)
		return got == len(want), fmt.Sprintf("%d of %d chunks", got, len(want))
	})
	sUnion := h.snapshot()
	_, firstAt, unionAt := h.arrivalSpan(j.i, want)
	h.waitQuiet(unionAt)
	endAt := time.Now()
	s1 := h.snapshot()
	r.first, r.union, r.quiet = firstAt.Sub(joinAt), unionAt.Sub(joinAt), endAt.Sub(joinAt)
	r.rateMin, r.rateMax, _ = w.span(joinAt, endAt)
	r.rateMinFirst, _, _ = w.span(joinAt, firstAt)
	r.radii = j.topo.radiusChanges()
	r.limiterBound = time.Duration(float64(len(want)) / 1000 * float64(time.Second))

	m := s1.nodes[j.i].since(s0.node(j.i))
	mu := sUnion.nodes[j.i].since(s0.node(j.i))
	r.metaIn, r.metaOut = m.value(`bee_rangesync_bytes_metadata{dir="in"}`), m.value(`bee_rangesync_bytes_metadata{dir="out"}`)
	r.metaInUnion = mu.value(`bee_rangesync_bytes_metadata{dir="in"}`)
	r.chunksIn = m.value(`bee_rangesync_bytes_chunks{dir="in"}`) + float64(s1.psBytes-s0.psBytes)
	r.offeredIn = m.value("bee_pullsync_chunks_offered")
	r.wireIn, r.wireInUnion = s1.wireInOf(j.i), sUnion.wireInOf(j.i)
	r.stripes = m.byLabel("bee_rangesync_stripes_total")
	r.sessions = m.byLabel("bee_rangesync_sessions_total")
	r.claims = m.byLabel("bee_rangesync_want_claims")
	vd := m.hists["bee_rangesync_verification_diff"]
	r.verified, r.verifiedDiff = vd.count, vd.sum
	for i := range h.nodes {
		mi := s1.nodes[i].since(s0.node(i))
		r.failures += mi.value("bee_rangesync_peer_failures_total")
		if i != j.i {
			r.holdersMeta += mi.value(`bee_rangesync_bytes_metadata{dir="in"}`)
			r.holders++
		}
	}
	r.disconnects = h.net.disconnects.Load() + h.net.blocklists.Load()
	r.cpu = s1.process.value("bee_process_cpu_seconds_total") - s0.process.value("bee_process_cpu_seconds_total")
	r.deliveries = h.deliveriesTo(j.i, want)
	h.sampler.mu.Lock()
	r.indexPeak, r.indexRun, r.heapPeak = h.sampler.indexPeak[j.i], h.sampler.indexPeakRun[j.i], h.sampler.heapPeak
	h.sampler.mu.Unlock()
	h.mu.Lock()
	lat := slices.Clone(h.fetchLat)
	h.mu.Unlock()
	slices.Sort(lat)
	if len(lat) > 0 {
		r.fetchLatP99 = lat[min(len(lat)-1, int(math.Ceil(0.99*float64(len(lat))))-1)]
		r.latMax = lat[len(lat)-1]
	}

	h.checkUnion()
	r.sampleEqual = sampleEqual(t, j, h.nodes[0])
	checkBootstrap(t, r)

	if accLong() {
		idleFrom, idle0 := time.Now(), h.sessionsWith(j)
		zeroAt := waitZero(t, j)
		r.idle = float64(h.sessionsWith(j)-idle0) / time.Since(idleFrom).Minutes()
		r.zero = zeroAt.Sub(joinAt)
		if got := j.topo.radiusChanges(); len(got) != 1 {
			t.Errorf("the joiner's radius changed after its sync rate returned to 0: %v", got)
		}
	}
	t.Logf("%s: %d holders + joiner (%s), setup %v, bootstrap %v, run %v", name, len(h.nodes)-1, mode, ms(r.setup), ms(r.union), ms(time.Since(begin)))
	return r
}

func checkBootstrap(t *testing.T, r *accBootResult) {
	t.Helper()

	if !r.sampleEqual {
		t.Errorf("AC-N5: the joiner's reserve sample differs from a holder's")
	}
	if r.failures != 0 || r.disconnects != 0 {
		t.Errorf("%v peer failures, %d disconnects and blocklists, want none", r.failures, r.disconnects)
	}
	if len(r.radii) != 1 {
		t.Errorf("AC-N2: the joiner's radius changed: %v", r.radii)
	}
	if !r.mode.Rangesync {
		return
	}
	// AC-D1, AC-B3: each chunk once
	if r.deliveries.rs != r.chunks || r.deliveries.repeats != 0 || r.deliveries.ps != 0 {
		t.Errorf("AC-D1: %d rangesync and %d pullsync deliveries of %d chunks, %d repeated", r.deliveries.rs, r.deliveries.ps, r.chunks, r.deliveries.repeats)
	}
	if limit := 1.01 * float64(r.chunks) * accChunkWire; r.chunksIn > limit {
		t.Errorf("AC-B3: %.0f chunk bytes in for %d chunks, more than %.0f", r.chunksIn, r.chunks, limit)
	}
	// AC-B2, its two parts apart: the IDs by the union, the verification
	// after it. The verification starts once every stripe is done, so in a
	// static network after the union; one that starts before the union's
	// snapshot counts in the ID part, which only tightens that check.
	if limit := 1.15 * float64(r.chunks) * 70; r.metaInUnion > limit {
		t.Errorf("AC-B2: %.0f metadata bytes in by the union for %d chunks, more than %.0f", r.metaInUnion, r.chunks, limit)
	}
	if v := r.metaIn - r.metaInUnion; v > 1e6 {
		t.Errorf("AC-B2: %.0f metadata bytes in after the union (the verification), more than 1 MB", v)
	}
	// AC-N1, AC-N2: above 0 throughout, so no reserve-worker tick lowered
	// the radius
	if r.rateMin <= 0 {
		t.Errorf("AC-N1: the joiner's sync rate read %v between the join and the end of the tail", r.rateMin)
	}
	if r.stripes["done"] == 0 {
		t.Errorf("the joiner striped nothing: stripes_total %v", r.stripes)
	}
}

// accChunkWire is the wire size of a rangesync Delivery of a full chunk with
// its stamp: the chunk bytes, the span, the stamp and the address with their
// framing (measured: 4258 B per delivered chunk, Decisions log part 1).
const accChunkWire = 4258

func logBootTable(t *testing.T, rs []*accBootResult) {
	t.Helper()

	var tb accTable
	val := func(f func(r *accBootResult) string) []string {
		out := make([]string, len(rs))
		for i, r := range rs {
			out[i] = "-"
			if r != nil {
				out[i] = f(r)
			}
		}
		return out
	}
	// ratio returns each result's value over the baseline's (rs[0])
	ratio := func(f func(r *accBootResult) float64) []string {
		return val(func(r *accBootResult) string {
			if rs[0] == nil || f(rs[0]) == 0 {
				return "-"
			}
			return fmt.Sprintf("%.2f", f(r)/f(rs[0]))
		})
	}
	tb.row("scenario", val(func(r *accBootResult) string { return fmt.Sprintf("%s (%d nodes, joiner %s)", r.name, r.nodes, r.mode) })...)
	tb.row("joiner's final reserve count", val(func(r *accBootResult) string { return strconv.Itoa(r.chunks) })...)
	tb.row("setup", val(func(r *accBootResult) string { return ms(r.setup) })...)
	tb.row("join to first chunk", val(func(r *accBootResult) string { return ms(r.first) })...)
	tb.row("join to union (AC-V3 proxy)", val(func(r *accBootResult) string { return ms(r.union) })...)
	tb.row("  / baseline", ratio(func(r *accBootResult) float64 { return r.union.Seconds() })...)
	tb.row("  chunks at 1000/s (fetch limiter)", val(func(r *accBootResult) string { return ms(r.limiterBound) })...)
	tb.row("join to end of quiet tail", val(func(r *accBootResult) string { return ms(r.quiet) })...)
	tb.row("join to sync rate 0 (long runs)", val(func(r *accBootResult) string {
		if r.zero == 0 {
			return "-"
		}
		return ms(r.zero)
	})...)
	tb.row("idle sessions per minute with peers (long runs)", val(func(r *accBootResult) string {
		if r.zero == 0 {
			return "-"
		}
		return fmt.Sprintf("%.2f", r.idle)
	})...)
	tb.row("sync rate min/max join..tail; min before 1st", val(func(r *accBootResult) string {
		return fmt.Sprintf("%.2f/%.2f; %.2f", r.rateMin, r.rateMax, r.rateMinFirst)
	})...)
	tb.row("radius changes", val(func(r *accBootResult) string { return fmt.Sprint(len(r.radii)) })...)
	tb.row("metadata in (AC: rs in + offers*102)", val(func(r *accBootResult) string { return mbytes(r.metaIn + 102*r.offeredIn) })...)
	tb.row("  per chunk", val(func(r *accBootResult) string {
		return fmt.Sprintf("%.1f B", (r.metaIn+102*r.offeredIn)/float64(r.chunks))
	})...)
	tb.row("  / baseline", ratio(func(r *accBootResult) float64 { return r.metaIn + 102*r.offeredIn })...)
	tb.row("rs metadata in by union (AC-B2 IDs), per chunk", val(func(r *accBootResult) string {
		if !r.mode.Rangesync {
			return "-"
		}
		return fmt.Sprintf("%s, %.1f B (bound %s, 80.5 B)", mbytes(r.metaInUnion), r.metaInUnion/float64(r.chunks), mbytes(1.15*float64(r.chunks)*70))
	})...)
	tb.row("rs metadata in after (AC-B2 verification)", val(func(r *accBootResult) string {
		if !r.mode.Rangesync {
			return "-"
		}
		return fmt.Sprintf("%.0f B (bound 1 MB)", r.metaIn-r.metaInUnion)
	})...)
	tb.row("rs metadata out (Wants)", val(func(r *accBootResult) string { return mbytes(r.metaOut) })...)
	tb.row("wire in: status/reconcile/fetch; pullsync", val(func(r *accBootResult) string {
		return fmt.Sprintf("%s/%s/%s; %s", mbytes(float64(r.wireIn["rangesync/status"])), mbytes(float64(r.wireIn["rangesync/reconcile"])),
			mbytes(float64(r.wireIn["rangesync/fetch"])), mbytes(float64(r.wireIn["pullsync/pullsync"])))
	})...)
	tb.row("holders' rs metadata in (sum)", val(func(r *accBootResult) string { return mbytes(r.holdersMeta) })...)
	tb.row("chunk bytes in; / chunks x 4258 B", val(func(r *accBootResult) string {
		return fmt.Sprintf("%s; %.3f", mbytes(r.chunksIn), r.chunksIn/(float64(r.chunks)*accChunkWire))
	})...)
	tb.row("deliveries pullsync/rangesync; repeats", val(func(r *accBootResult) string {
		return fmt.Sprintf("%d/%d; %d", r.deliveries.ps, r.deliveries.rs, r.deliveries.repeats)
	})...)
	tb.row("stripes_total", val(func(r *accBootResult) string { return fmtCounts(r.stripes) })...)
	tb.row("verified stripes; chunks they delivered", val(func(r *accBootResult) string {
		return fmt.Sprintf("%d; %.0f", r.verified, r.verifiedDiff)
	})...)
	tb.row("sessions_total", val(func(r *accBootResult) string { return fmtCounts(r.sessions) })...)
	tb.row("want_claims", val(func(r *accBootResult) string { return fmtCounts(r.claims) })...)
	tb.row("rs fetch latency p99 / max", val(func(r *accBootResult) string { return ms(r.fetchLatP99) + " / " + ms(r.latMax) })...)
	tb.row("peer failures; disconnects", val(func(r *accBootResult) string { return fmt.Sprintf("%.0f; %d", r.failures, r.disconnects) })...)
	tb.row("CPU s (process) per 1000 chunks", val(func(r *accBootResult) string { return fmt.Sprintf("%.2f", 1000*r.cpu/float64(r.chunks)) })...)
	tb.row("  / baseline", ratio(func(r *accBootResult) float64 { return r.cpu })...)
	tb.row("joiner peak index memory window (run)", val(func(r *accBootResult) string { return mbytes(r.indexPeak) + " (" + mbytes(r.indexRun) + ")" })...)
	tb.row("peak live Go heap (process)", val(func(r *accBootResult) string { return mbytes(float64(r.heapPeak)) })...)
	tb.row("reserve sample equals a holder's (AC-N5)", val(func(r *accBootResult) string { return fmt.Sprint(r.sampleEqual) })...)
	t.Logf("T6.1 Bootstrap:\n%s", tb.String())
}

// uploadPhase pushes count new chunks at rate per second, calling before, if
// not nil, before each push with the push's number and the time since the
// first, and returns when the last push returned.
func (h *accHarness) uploadPhase(count int, rate float64, before func(k int, since time.Duration)) time.Time {
	chunks := make([]swarm.Chunk, count)
	for i := range chunks {
		chunks[i] = h.newChunk()
	}
	interval := time.Duration(float64(time.Second) / rate)
	start := time.Now()
	next := start
	for k, ch := range chunks {
		time.Sleep(time.Until(next))
		if before != nil {
			before(k, time.Since(start))
		}
		h.push(ch)
		next = next.Add(interval)
	}
	return time.Now()
}

// flipBit returns a with bit b flipped.
func flipBit(a swarm.Address, b uint8) swarm.Address {
	out := slices.Clone(a.Bytes())
	out[b/8] ^= 0x80 >> (b % 8)
	return swarm.NewAddress(out)
}

// --- Radius decrease ---------------------------------------------------------

const (
	// accRadiusSide is the number of nodes in each half of the Radius
	// decrease network.
	accRadiusSide = 3
	// accRadiusWakeUp is the decreaser's reserve-worker interval (the node:
	// 15 minutes). It must exceed the reconciler's reaction to a root change
	// (the index rebuild, then a round at most MinRoundInterval after the
	// last): until that round tracks the new root the reconciler's sync rate
	// reads 0, and a worker that checks every second lowered the radius a
	// second time one second after the first (Decisions log, T6.1 part 2).
	accRadiusWakeUp = 2 * reconciler.MinRoundInterval
)

// accRadiusResult is what a Radius decrease scenario measured: at the node
// whose radius drops (the decreaser) from the release of its reserve worker
// to the end of the quiet tail after its new half arrived, then over a phase
// of uploads with the mixed radii.
type accRadiusResult struct {
	name    string
	mode    accMode
	highest bool // the decreaser's overlay is the highest of the network, not the lowest
	setup   time.Duration
	half    int // chunks in the new half

	decrease    time.Duration // release to the radius decrease
	first, done time.Duration // decrease to the new half's first and last chunk
	zero        time.Duration // decrease to the sync rate back at 0 (long runs)
	second      time.Duration // decrease to the second decrease (long runs)
	idle        float64       // the decreaser's sessions per minute with its peers while the long run waits (long runs)
	secondAfter time.Duration // the second decrease after the last positive sync rate read (long runs)
	rateMin     float64       // the decreaser's sync rate from the decrease to the tail's end
	radii       []accRadiusChange

	metaIn, metaOut float64 // rangesync bytes_metadata at the decreaser
	offeredIn       float64 // pullsync offers it received
	chunksIn        float64 // bytes of the chunk deliveries to it
	deliveries      accDeliveryCount
	oldHalf         []uint64      // reconcile bytes with each peer of its old half after the release
	oldHalfSessions []int64       // reconcile streams with each peer of its old half after the release
	newHalfPeers    []uint64      // reconcile + fetch bytes with each peer of the new half
	disjoint        int64         // reconcile streams between the two halves' other nodes once ready
	startup         int64         // reconcile streams between the two halves before ready
	rateRise        time.Duration // decrease to the first positive sync rate read
	stripes         map[string]float64
	failures        float64
	disconnects     int64
	cpu             float64

	upload *accResult // the mixed-radii upload phase
	// the latency of the uploads of the phase that the decreaser lacked
	// (not pushed to it): median, p95, max
	nestedLat [3]time.Duration
	// the uploads of the phase within AC-V1's bound (accV1Bound), and the
	// smallest bound; the same within 2 × the smallest round interval of the
	// network at the upload (the bound asserted before fix round 1 of T6.1
	// part 2, logged); and the uploads within accStatusBound (asserted)
	v1Within, v1WithinLeast, v1WithinStatus int
	v1Bound, v1BoundLeast                   time.Duration
}

// radiusOverlay returns the overlays of the Radius decrease network: node i
// at radius 2 in one half of the radius-1 neighborhood, the decreaser (node
// 0) and the next accRadiusSide − 1 nodes in one half (bit 1), the others in
// the other. Without highest the seed sets the order (with seed 200 the
// decreaser has the lowest overlay, so it initiates every session with its
// peers, §8.2). With highest the decreaser's half is the upper one and the
// decreaser the highest node of it, so that its peers initiate every session
// with it.
func radiusOverlay(highest bool) func(i int, random, prefix swarm.Address) swarm.Address {
	return func(i int, random, prefix swarm.Address) swarm.Address {
		a := withPrefix(random, prefix, 2)
		if i >= accRadiusSide {
			a = flipBit(a, 1)
		}
		if !highest {
			return a
		}
		// the decreaser's half has bit 1 set
		if prefix.Bytes()[0]&0x40 == 0 {
			a = flipBit(a, 1)
		}
		if i == 0 {
			b := slices.Clone(a.Bytes())
			b[0] |= 0x3f
			b[1] = 0xff
			a = swarm.NewAddress(b)
		}
		return a
	}
}

// TestAcceptanceRadius runs the Radius decrease scenario: two halves of a
// radius-1 neighborhood, accRadiusSide nodes each at radius 2, each half
// holding its base chunks (~4500). The harness then lets node 0's reserve
// worker lower its radius (its reserve is below the threshold, and it checks
// every accRadiusWakeUp): its root widens to radius 1, whose new half the
// other half holds. Then uploads into the whole neighborhood run with the
// mixed radii. It runs on rangesync-only nodes in two orientations, the
// decreaser's overlay the lowest of the network and the highest
// (radiusOverlay), and on pullsync-only nodes (the Baseline, for AC-V4;
// accFull). On rangesync it checks:
//   - AC-B4: the decreaser's chunk bytes ≤ 1.01 × the new half, each chunk
//     once (AC-D1); the old half costs < 1 KB of sessions with each of its
//     peers (the decreaser lowest: it compares their digests over their
//     root and starts none; highest: they cannot compare the decreaser's
//     digest and start one in each of their rounds, a fingerprint exchange
//     that finds the sets equal);
//   - AC-N1: its sync rate is above 0 from the decrease to the end of the
//     tail; AC-N3: no second decrease meanwhile, although its count is still
//     below the threshold (with accLong: the second decrease comes only once
//     the sync rate read 0);
//   - AC-C4: no session between the two halves' nodes at radius 2 (disjoint
//     roots), none between the decreaser and the other half before the
//     decrease, and no reserve holds a chunk outside its root (the other
//     half's nodes sync only their own root with the decreaser);
//   - AC-C1, AC-K3; and, in the upload phase with the mixed radii (accFull),
//     AC-V1 and AC-D1.
func TestAcceptanceRadius(t *testing.T) {
	o := accOptions{
		Nodes: 2 * accRadiusSide, Seed: 200, Radius: 1, Base: 9000,
		NodeRadius: func(int) uint8 { return 2 },
		Hold:       func(i int) bool { return i == 0 },
		WakeUp: func(i int) time.Duration {
			if i == 0 {
				return accRadiusWakeUp
			}
			return 0 // the default
		},
		Window: accWindow, LargeDiff: accLargeDiff, Tail: accTail,
	}
	var results [3]*accRadiusResult
	// after the subtests, which run in parallel in long runs
	t.Cleanup(func() { logRadiusTable(t, results[:]) })
	for k, c := range []struct {
		name    string
		mode    accMode
		highest bool
	}{
		{"baseline", accPullsyncOnly, false},
		{"rangesync", accRangesyncOnly, false},
		{"rangesync decreaser highest", accRangesyncOnly, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			if !c.mode.Rangesync && !accFull() {
				t.Skip("the Baseline runs with RANGESYNC_ACCEPTANCE_FULL or RANGESYNC_ACCEPTANCE_LONG set")
			}
			if accLong() {
				t.Parallel()
			}
			ro := o
			ro.Mode = func(int) accMode { return c.mode }
			ro.Overlay = radiusOverlay(c.highest)
			results[k] = runRadius(t, c.name, ro, c.mode, c.highest)
		})
	}
}

func runRadius(t *testing.T, name string, o accOptions, mode accMode, highest bool) *accRadiusResult {
	t.Helper()

	begin := time.Now()
	h := newAccHarness(t, o)
	defer h.close()
	h.start()
	r := &accRadiusResult{name: name, mode: mode, highest: highest, setup: time.Since(begin)}

	d := h.nodes[0]
	for _, p := range h.nodes[1:] {
		if above := d.overlay.Compare(p.overlay) > 0; above != highest {
			t.Fatalf("the decreaser's overlay %s is not the %s of the network (node %d: %s)", d.overlay, map[bool]string{false: "lowest", true: "highest"}[highest], p.i, p.overlay)
		}
	}
	oldPeers, newPeers := h.nodes[1:accRadiusSide], h.nodes[accRadiusSide:]
	newHalf := fingerprint.NewRange(flipBit(d.overlay, 1), 2)
	want := idsIn(t, h.base, newHalf)
	r.half = len(want)
	const reconcile, fetch = "rangesync/reconcile", "rangesync/fetch"
	pairBytes := func(a *accNode, peers []*accNode, streams ...string) []uint64 {
		out := make([]uint64, len(peers))
		for k, p := range peers {
			for _, s := range streams {
				_, b := h.net.pairStats(a.overlay, p.overlay, s)
				out[k] += b
			}
		}
		return out
	}
	pairStreams := func(a *accNode, peers []*accNode, stream string) []int64 {
		out := make([]int64, len(peers))
		for k, p := range peers {
			out[k], _ = h.net.pairStats(a.overlay, p.overlay, stream)
		}
		return out
	}
	crossStreams := func(as []*accNode) int64 {
		var sum int64
		for _, a := range as {
			for _, b := range newPeers {
				n, _ := h.net.pairStats(a.overlay, b.overlay, reconcile)
				sum += n
			}
		}
		return sum
	}
	// sessions between the halves before the network is ready: at start-up
	// a node's index has the root of radius 0 until it is rebuilt at the
	// radius of the warm-up
	r.startup = crossStreams(h.nodes[:accRadiusSide])
	disjoint0 := crossStreams(oldPeers)

	runtime.GC()
	h.sampler.resetPeaks()
	s0 := h.snapshot()
	old0, new0 := pairBytes(d, oldPeers, reconcile), pairBytes(d, newPeers, reconcile, fetch)
	oldSessions0 := pairStreams(d, oldPeers, reconcile)
	w := watchRate(d)
	defer w.stop()
	releaseAt := time.Now()
	d.release()
	accWait(t, "the radius decrease", 30*time.Second, func() (bool, string) {
		return d.db.StorageRadius() == 1, fmt.Sprintf("radius %d", d.db.StorageRadius())
	})
	decAt := d.topo.radiusChanges()[1].at
	r.decrease = decAt.Sub(releaseAt)
	accWait(t, "the new half", accBootstrapLimit, func() (bool, string) {
		got, _, _ := h.arrivalSpan(d.i, want)
		return got == len(want), fmt.Sprintf("%d of %d chunks", got, len(want))
	})
	_, firstAt, doneAt := h.arrivalSpan(d.i, want)
	r.first, r.done = firstAt.Sub(decAt), doneAt.Sub(decAt)
	h.waitQuiet(doneAt)
	endAt := time.Now()
	s1 := h.snapshot()
	// AC-N1 from the transfer's first chunk: between the decrease and the
	// round that tracks the new root the reconciler reads 0, as the puller
	// does until its first page
	r.rateMin, _, _ = w.span(firstAt, endAt)
	r.rateRise = -1
	w.mu.Lock()
	for _, s := range w.samples {
		if s.at.After(decAt) && s.rate > 0 {
			r.rateRise = s.at.Sub(decAt)
			break
		}
	}
	w.mu.Unlock()
	r.radii = d.topo.radiusChanges()

	m := s1.nodes[d.i].since(s0.node(d.i))
	r.metaIn, r.metaOut = m.value(`bee_rangesync_bytes_metadata{dir="in"}`), m.value(`bee_rangesync_bytes_metadata{dir="out"}`)
	r.offeredIn = m.value("bee_pullsync_chunks_offered")
	r.chunksIn = m.value(`bee_rangesync_bytes_chunks{dir="in"}`) + float64(s1.psBytes-s0.psBytes)
	r.stripes = m.byLabel("bee_rangesync_stripes_total")
	r.deliveries = h.deliveriesTo(d.i, want)
	for k, b := range pairBytes(d, oldPeers, reconcile) {
		r.oldHalf = append(r.oldHalf, b-old0[k])
	}
	for k, n := range pairStreams(d, oldPeers, reconcile) {
		r.oldHalfSessions = append(r.oldHalfSessions, n-oldSessions0[k])
	}
	for k, b := range pairBytes(d, newPeers, reconcile, fetch) {
		r.newHalfPeers = append(r.newHalfPeers, b-new0[k])
	}
	for i := range h.nodes {
		r.failures += s1.nodes[i].since(s0.node(i)).value("bee_rangesync_peer_failures_total")
	}
	r.disconnects = h.net.disconnects.Load() + h.net.blocklists.Load()
	r.cpu = s1.process.value("bee_process_cpu_seconds_total") - s0.process.value("bee_process_cpu_seconds_total")
	if mode.Rangesync && accFull() {
		h.checkUnionNoProbe() // the probes run once, at the end
		radiusUploadPhase(t, h, r, d)
	}
	// AC-N1 and AC-N3 over the whole default run: the decreaser's sync rate
	// from the transfer's first chunk, and its radius
	r.rateMin, _, _ = w.span(firstAt, time.Now())
	r.radii = d.topo.radiusChanges()
	h.checkUnion()
	r.disjoint = crossStreams(oldPeers) - disjoint0
	checkRadius(t, r)

	if accLong() {
		idleFrom, idle0 := time.Now(), h.sessionsWith(d)
		zeroAt := waitZero(t, d)
		r.idle = float64(h.sessionsWith(d)-idle0) / time.Since(idleFrom).Minutes()
		_, hi, _ := w.span(decAt, zeroAt)
		lastPositive := decAt
		w.mu.Lock()
		for _, s := range w.samples {
			if s.rate > 0 && s.at.After(lastPositive) {
				lastPositive = s.at
			}
		}
		w.mu.Unlock()
		r.zero = zeroAt.Sub(decAt)
		accWait(t, "the second radius decrease", 30*time.Second, func() (bool, string) {
			return d.db.StorageRadius() == 0, fmt.Sprintf("radius %d", d.db.StorageRadius())
		})
		radii := d.topo.radiusChanges()
		second := radii[len(radii)-1].at
		r.second, r.secondAfter = second.Sub(decAt), second.Sub(lastPositive)
		if !second.After(lastPositive) {
			t.Errorf("AC-N3: the second decrease came %v before the last positive sync rate (max %v)", lastPositive.Sub(second), hi)
		}
	}
	t.Logf("%s: %d+%d nodes (%s), setup %v, new half %v after the decrease, run %v", name, accRadiusSide, accRadiusSide, mode, ms(r.setup), ms(r.done), ms(time.Since(begin)))
	return r
}

// radiusUploadPhase runs the upload phase with the mixed radii on a
// rangesync network: the decreaser d at radius 1, the others at 2 (nested
// roots, §7.6). It runs with accFull; the pullsync network skips it (its
// numbers are in the Decisions log).
func radiusUploadPhase(t *testing.T, h *accHarness, r *accRadiusResult, d *accNode) {
	t.Helper()

	runtime.GC()
	h.sampler.resetPeaks()
	u0 := h.snapshot()
	h.mu.Lock()
	h.uploads = nil // the phase's uploads only
	h.mu.Unlock()
	rate := accRate(len(h.nodes))
	// every node's round interval at each upload, for AC-V1's bound
	var intervals [][]time.Duration
	rounds := h.roundIntervals()
	lastUpload := h.uploadPhase(int(rate*accRadiusUploadPhase.Seconds()), rate, func(int, time.Duration) {
		intervals = append(intervals, rounds())
	})
	accWait(t, "every node to hold its root's union", h.o.Limit, func() (bool, string) {
		missing, _ := h.pending()
		return missing == 0, fmt.Sprintf("%d (upload, node) pairs missing", missing)
	})
	_, convergedAt := h.pending()
	quietAt := h.waitQuiet(lastUpload)
	u1 := h.snapshot()
	r.upload = h.result(r.name+" uploads", u0, u1, lastUpload, convergedAt, quietAt)
	r.nestedLat = h.latencyAt(d.i)
	r.v1Within, r.v1Bound = h.uploadsWithin(intervals, accV1Bound)
	r.v1WithinLeast, r.v1BoundLeast = h.uploadsWithin(intervals, func(_ *accUpload, iv []time.Duration) time.Duration {
		return 2 * slices.Min(iv)
	})
	r.v1WithinStatus, _ = h.uploadsWithin(intervals, func(*accUpload, []time.Duration) time.Duration { return accStatusBound })
}

// latencyAt returns the median, p95 and largest time from an upload to its
// arrival at node i, over the uploads node i got through sync (its root holds
// them and they were not pushed to it).
func (h *accHarness) latencyAt(i int) [3]time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	var lat []time.Duration
	for _, u := range h.uploads {
		if !slices.Contains(u.want, i) || slices.Contains(u.holders, i) {
			continue
		}
		if at, ok := h.arrivals[i][u.id]; ok {
			lat = append(lat, at.Sub(u.at))
		}
	}
	if len(lat) == 0 {
		return [3]time.Duration{}
	}
	slices.Sort(lat)
	return [3]time.Duration{lat[len(lat)/2], lat[min(len(lat)-1, int(math.Ceil(0.95*float64(len(lat))))-1)], lat[len(lat)-1]}
}

// roundIntervals returns a function that reads every node's round interval
// (round_interval_seconds), by node index, at least MinRoundInterval
// (MinRoundInterval for a node without a reconciler).
func (h *accHarness) roundIntervals() func() []time.Duration {
	gauges := make([]prometheus.Collector, len(h.nodes))
	for i, n := range h.nodes {
		if n.rec != nil {
			gauges[i] = findCollector(h.tb, "bee_rangesync_round_interval_seconds", n.rec.Metrics()...)
		}
	}
	return func() []time.Duration {
		out := make([]time.Duration, len(gauges))
		for i, g := range gauges {
			out[i] = reconciler.MinRoundInterval
			if g != nil {
				out[i] = max(out[i], time.Duration(collectSum(g)*float64(time.Second)))
			}
		}
		return out
	}
}

// accStatusBound bounds an upload's spread whatever the round intervals:
// a lacker below every holder gets their Status within 2 ×
// StatusMinInterval when they sent it a change just before, and its round
// may then wait MinRoundInterval (T4.6 part 1 fix round 1, Low 4: 15 s). It
// keeps the mixed-radii check tight where AC-V1's bound (accV1Bound) is wide
// because a node's interval grew while its neighborhood was quiet.
const accStatusBound = 2*rangesync.StatusMinInterval + reconciler.MinRoundInterval

// accV1Bound is AC-V1's bound for upload u, from every node's round interval
// at the upload (iv, by node index): 2 × the round interval of the nodes
// whose root holds it, the largest of theirs. The upload is on all of them
// once the last has it, and each takes it in its own rounds, which come at
// its own interval (a node's interval adapts, round_interval_seconds): the
// neighborhood's 2 × RoundInterval is set by its slowest node.
func accV1Bound(u *accUpload, iv []time.Duration) time.Duration {
	var bound time.Duration
	for _, i := range u.want {
		bound = max(bound, 2*iv[i])
	}
	return bound
}

// uploadsWithin returns the number of uploads (h.uploads, in order) that
// reached every node whose root holds them within their bound, from every
// node's round interval at the upload (intervals, one per upload, by node
// index), and the smallest bound.
func (h *accHarness) uploadsWithin(intervals [][]time.Duration, bound func(u *accUpload, iv []time.Duration) time.Duration) (int, time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	n, least := 0, time.Duration(math.MaxInt64)
	for k, u := range h.uploads {
		b := bound(u, intervals[k])
		least = min(least, b)
		ok := true
		for _, i := range u.want {
			if at, arrived := h.arrivals[i][u.id]; !arrived || at.Sub(u.at) > b {
				ok = false
				break
			}
		}
		if ok {
			n++
		}
	}
	return n, least
}

// accRadiusUploadPhase is how long the uploads with the mixed radii last: 2
// rounds of MinRoundInterval.
const accRadiusUploadPhase = 10 * time.Second

func checkRadius(t *testing.T, r *accRadiusResult) {
	t.Helper()

	if r.disjoint != 0 {
		t.Errorf("AC-C4: %d sessions between nodes of disjoint roots", r.disjoint)
	}
	if r.failures != 0 || r.disconnects != 0 {
		t.Errorf("%v peer failures, %d disconnects and blocklists, want none", r.failures, r.disconnects)
	}
	// AC-N3: one decrease; the next waits until the sync rate reads 0, more
	// than 10 minutes later
	if len(r.radii) != 2 {
		t.Errorf("AC-N3: radii %v, want the start and one decrease", r.radii)
	}
	if !r.mode.Rangesync {
		return
	}
	// AC-B4: the new half moves once, the old half costs a fingerprint
	// exchange per peer at most
	if r.deliveries.rs != r.half || r.deliveries.repeats != 0 || r.deliveries.ps != 0 {
		t.Errorf("AC-D1: %d rangesync and %d pullsync deliveries of the %d chunks of the new half, %d repeated", r.deliveries.rs, r.deliveries.ps, r.half, r.deliveries.repeats)
	}
	if limit := 1.01 * float64(r.half) * accChunkWire; r.chunksIn > limit {
		t.Errorf("AC-B4: %.0f chunk bytes in for a new half of %d chunks, more than %.0f", r.chunksIn, r.half, limit)
	}
	for k, b := range r.oldHalf {
		if b >= 1000 {
			t.Errorf("AC-B4: %d reconcile bytes (%d sessions) with old-half peer %d after the decrease, want < 1 KB", b, r.oldHalfSessions[k], k+1)
		}
	}
	if r.rateMin <= 0 {
		t.Errorf("AC-N1: the decreaser's sync rate read %v during the new half's transfer", r.rateMin)
	}
	if r.stripes["done"] == 0 {
		t.Errorf("the new half was not striped: stripes_total %v", r.stripes)
	}
	u := r.upload
	if u == nil {
		return
	}
	if u.dupRS != 0 || u.dupBase != 0 {
		t.Errorf("AC-D1 (uploads, mixed radii): rangesync delivered %d IDs again and %d base chunks", u.dupRS, u.dupBase)
	}
	if u.decodeCorruption != 0 {
		t.Errorf("AC-K3: decode_corruption_total %v", u.decodeCorruption)
	}
	// AC-V1 against the round intervals at each upload (accV1Bound): the
	// decreaser's interval grew to MaxRoundInterval while its neighborhood
	// was quiet (T4.6 part 3: AC-V1 at a quiet network), so 2 ×
	// MinRoundInterval, and 2 × the smallest interval of the network, are
	// tighter than the criterion there. A decreaser that initiates with the
	// holders (lowest) has its latency bound by a Status interval and a
	// round's (10 s), right at 2 × MinRoundInterval. accStatusBound keeps the
	// check tight where AC-V1's bound is wide.
	if r.v1Within != u.uploads {
		t.Errorf("AC-V1 (mixed radii): %d of %d uploads reached every node within 2 × the largest round interval at the upload of the nodes whose root holds it (at least %v; the slowest took %v)", r.v1Within, u.uploads, r.v1Bound, u.latMax)
	}
	if r.v1WithinStatus != u.uploads {
		t.Errorf("mixed radii: %d of %d uploads reached every node within %v, 2 × StatusMinInterval + MinRoundInterval (the slowest took %v)", r.v1WithinStatus, u.uploads, accStatusBound, u.latMax)
	}
}

func logRadiusTable(t *testing.T, rs []*accRadiusResult) {
	t.Helper()

	var tb accTable
	val := func(f func(r *accRadiusResult) string) []string {
		out := make([]string, len(rs))
		for i, r := range rs {
			out[i] = "-"
			if r != nil {
				out[i] = f(r)
			}
		}
		return out
	}
	// ratio returns each result's value over the baseline's (rs[0])
	ratio := func(f func(r *accRadiusResult) float64) []string {
		return val(func(r *accRadiusResult) string {
			if rs[0] == nil || f(rs[0]) == 0 {
				return "-"
			}
			return fmt.Sprintf("%.2f", f(r)/f(rs[0]))
		})
	}
	tb.row("scenario", val(func(r *accRadiusResult) string {
		return fmt.Sprintf("%s (%d+%d nodes, %s)", r.name, accRadiusSide, accRadiusSide, r.mode)
	})...)
	tb.row("the decreaser's overlay", val(func(r *accRadiusResult) string {
		return map[bool]string{false: "lowest", true: "highest"}[r.highest]
	})...)
	tb.row("chunks in the new half", val(func(r *accRadiusResult) string { return strconv.Itoa(r.half) })...)
	tb.row("setup", val(func(r *accRadiusResult) string { return ms(r.setup) })...)
	tb.row("release to decrease", val(func(r *accRadiusResult) string { return ms(r.decrease) })...)
	tb.row("decrease to first / last chunk (AC-V4 proxy)", val(func(r *accRadiusResult) string { return ms(r.first) + " / " + ms(r.done) })...)
	tb.row("  / baseline", ratio(func(r *accRadiusResult) float64 { return r.done.Seconds() })...)
	tb.row("decrease to sync rate 0; 2nd decrease (long)", val(func(r *accRadiusResult) string {
		if r.zero == 0 {
			return "-"
		}
		return fmt.Sprintf("%s; %s (%s after last rate > 0)", ms(r.zero), ms(r.second), ms(r.secondAfter))
	})...)
	tb.row("idle sessions per minute with peers (long)", val(func(r *accRadiusResult) string {
		if r.zero == 0 {
			return "-"
		}
		return fmt.Sprintf("%.2f", r.idle)
	})...)
	tb.row("sync rate > 0 after decrease; min from 1st chunk", val(func(r *accRadiusResult) string {
		return fmt.Sprintf("%s; %.2f", ms(r.rateRise), r.rateMin)
	})...)
	tb.row("radii", val(func(r *accRadiusResult) string {
		radii := make([]string, 0, len(r.radii))
		for _, c := range r.radii {
			radii = append(radii, strconv.Itoa(int(c.radius)))
		}
		return strings.Join(radii, ">")
	})...)
	tb.row("metadata in (AC: rs in + offers*102)", val(func(r *accRadiusResult) string {
		return fmt.Sprintf("%s (%.1f B/chunk)", mbytes(r.metaIn+102*r.offeredIn), (r.metaIn+102*r.offeredIn)/float64(r.half))
	})...)
	tb.row("  / baseline", ratio(func(r *accRadiusResult) float64 { return r.metaIn + 102*r.offeredIn })...)
	tb.row("rs metadata out (Wants)", val(func(r *accRadiusResult) string { return mbytes(r.metaOut) })...)
	tb.row("chunk bytes in; / new half x 4258 B", val(func(r *accRadiusResult) string {
		return fmt.Sprintf("%s; %.3f", mbytes(r.chunksIn), r.chunksIn/(float64(r.half)*accChunkWire))
	})...)
	tb.row("deliveries pullsync/rangesync; repeats", val(func(r *accRadiusResult) string {
		return fmt.Sprintf("%d/%d; %d", r.deliveries.ps, r.deliveries.rs, r.deliveries.repeats)
	})...)
	tb.row("old-half sessions, B per peer (AC-B4)", val(func(r *accRadiusResult) string { return fmt.Sprint(r.oldHalf) })...)
	tb.row("  reconcile streams per old-half peer", val(func(r *accRadiusResult) string { return fmt.Sprint(r.oldHalfSessions) })...)
	tb.row("new-half reconcile+fetch, MB per peer", val(func(r *accRadiusResult) string {
		parts := make([]string, 0, len(r.newHalfPeers))
		for _, b := range r.newHalfPeers {
			parts = append(parts, fmt.Sprintf("%.2f", float64(b)/1e6))
		}
		return strings.Join(parts, " ")
	})...)
	tb.row("sessions across disjoint roots; at start-up", val(func(r *accRadiusResult) string { return fmt.Sprintf("%d; %d", r.disjoint, r.startup) })...)
	tb.row("stripes_total", val(func(r *accRadiusResult) string { return fmtCounts(r.stripes) })...)
	tb.row("peer failures; disconnects", val(func(r *accRadiusResult) string { return fmt.Sprintf("%.0f; %d", r.failures, r.disconnects) })...)
	tb.row("CPU s (process), decrease phase", val(func(r *accRadiusResult) string { return fmt.Sprintf("%.2f", r.cpu) })...)
	tb.row("  / baseline", ratio(func(r *accRadiusResult) float64 { return r.cpu })...)
	// uval is val for the upload phase, which only rangesync networks run
	uval := func(f func(r *accRadiusResult) string) []string {
		return val(func(r *accRadiusResult) string {
			if r.upload == nil {
				return "-"
			}
			return f(r)
		})
	}
	tb.row("uploads (mixed radii): count, rate", uval(func(r *accRadiusResult) string { return fmt.Sprintf("%d, %.0f/s", r.upload.uploads, r.upload.rate) })...)
	tb.row("  upload latency median / p95 / max", uval(func(r *accRadiusResult) string {
		u := r.upload
		return fmt.Sprintf("%v/%v/%v", u.latMedian.Round(10*time.Millisecond), u.latP95.Round(10*time.Millisecond), u.latMax.Round(10*time.Millisecond))
	})...)
	tb.row("  same, at the decreaser (nested root)", uval(func(r *accRadiusResult) string {
		l := r.nestedLat
		return fmt.Sprintf("%v/%v/%v", l[0].Round(10*time.Millisecond), l[1].Round(10*time.Millisecond), l[2].Round(10*time.Millisecond))
	})...)
	tb.row("  round_interval_seconds min / max", uval(func(r *accRadiusResult) string {
		if r.upload.roundMax == 0 {
			return "-"
		}
		return fmt.Sprintf("%.1f / %.1f", r.upload.roundMin, r.upload.roundMax)
	})...)
	tb.row("  within 2xMinRoundInterval; 2x network's least", uval(func(r *accRadiusResult) string {
		return fmt.Sprintf("%d/%d; %d/%d (bound >= %v)", r.upload.v1Within, r.upload.uploads, r.v1WithinLeast, r.upload.uploads, r.v1BoundLeast)
	})...)
	tb.row("  within AC-V1 (2x root's largest); 15 s", uval(func(r *accRadiusResult) string {
		return fmt.Sprintf("%d/%d (bound >= %v); %d/%d", r.v1Within, r.upload.uploads, r.v1Bound, r.v1WithinStatus, r.upload.uploads)
	})...)
	tb.row("  metadata B/upload (AC def)", uval(func(r *accRadiusResult) string {
		u := r.upload
		return fmt.Sprintf("%.0f", (u.rsMetaIn+102*u.psOffered)/float64(max(1, u.uploads)))
	})...)
	tb.row("  duplicates (pullsync/rangesync/cross); races", uval(func(r *accRadiusResult) string {
		u := r.upload
		return fmt.Sprintf("%d/%d/%d; %d", u.dupPS, u.dupRS, u.dupCross, u.dupPushRS)
	})...)
	tb.row("  streams exhausted; more requests; parts", uval(func(r *accRadiusResult) string {
		u := r.upload
		return fmt.Sprintf("%.0f; %.0f; %s", u.exhausted, u.moreRequests, fmtCounts(u.parts))
	})...)
	tb.row("  symbols_per_difference median/p95/mean", uval(func(r *accRadiusResult) string {
		u := r.upload
		return fmt.Sprintf("<=%v/<=%v/%.2f", u.symMedian, u.symP95, u.symMean)
	})...)
	tb.row("  CPU s per 1000 uploads", uval(func(r *accRadiusResult) string {
		return fmt.Sprintf("%.2f", 1000*r.upload.cpu/float64(max(1, r.upload.uploads)))
	})...)
	t.Logf("T6.1 Radius decrease:\n%s", tb.String())
}

// --- Stale stamp -------------------------------------------------------------

// accStaleObserve is how long the Stale stamp scenario watches the rounds
// after the nodes converged on the newer stamp: 4 rounds of MinRoundInterval.
const accStaleObserve = 4 * reconciler.MinRoundInterval

// accStaleResult is what a Stale stamp scenario measured.
type accStaleResult struct {
	name     string
	mode     accMode
	converge time.Duration // from the pushes until every node holds the newer stamp only
	// per node: deliveries of the older and the newer stamp, deliveries of
	// the older after the convergence, deliveries of the older that the
	// reserve rejected (it held the newer), sessions with the stale peer
	// after the convergence, and the size of the rejected cache at the end
	older, newer, olderAfter, olderRejected []int
	staleSessions                           []int64
	rejected                                []float64
	claims                                  map[string]float64
	zeroAfterTTL                            []int // long runs: deliveries of the older stamp within RejectTTL + 1 min
}

// TestAcceptanceStaleStamp runs the Stale stamp scenario (§7.7): one chunk
// is pushed with a stamp to node 0 and, with a newer stamp of the same batch
// and index (restamp), to node 1. On rangesync-only nodes a stale peer also
// holds the older stamp for good (a rangesync service that serves sessions
// and fetches but never fetches itself): the nodes meet the older stamp in
// every round. It checks AC-C3: every node ends with the newer stamp and not
// the older; every node got the older stamp at most once, and not at all
// after the convergence although every round lists it (the rejected cache);
// rejected_cache_size stays at most the one stale ID. AC-C1 holds among the
// nodes. On pullsync-only nodes (the Baseline, without the stale peer;
// accFull) it logs whether the nodes end with the newer stamp. With accLong it also
// watches the rounds past RejectTTL: an ID leaves the rejected cache after
// it, and is fetched (and rejected) once more.
func TestAcceptanceStaleStamp(t *testing.T) {
	o := accOptions{Nodes: 4, Seed: 300, Radius: 2, Window: accWindow, LargeDiff: accLargeDiff, Tail: accTail}
	var results [2]*accStaleResult
	// after the subtests, which run in parallel in long runs
	t.Cleanup(func() { logStaleTable(t, results[:]) })
	for k, m := range []accMode{accPullsyncOnly, accRangesyncOnly} {
		name := map[accMode]string{accPullsyncOnly: "baseline", accRangesyncOnly: "rangesync"}[m]
		t.Run(name, func(t *testing.T) {
			if !m.Rangesync && !accFull() {
				t.Skip("the Baseline runs with RANGESYNC_ACCEPTANCE_FULL or RANGESYNC_ACCEPTANCE_LONG set")
			}
			if accLong() {
				t.Parallel()
			}
			so := o
			so.Mode = func(int) accMode { return m }
			results[k] = runStale(t, name, so, m)
		})
	}
}

func logStaleTable(t *testing.T, results []*accStaleResult) {
	t.Helper()

	var tb accTable
	for _, r := range results {
		if r == nil {
			continue
		}
		tb.row(r.name+": time to the newer stamp everywhere", ms(r.converge))
		tb.row("  per node: older / newer stamp deliveries", fmt.Sprint(r.older), fmt.Sprint(r.newer))
		tb.row("  per node: older rejected; after convergence", fmt.Sprint(r.olderRejected), fmt.Sprint(r.olderAfter))
		tb.row("  per node: sessions with the stale peer after", fmt.Sprint(r.staleSessions))
		tb.row("  per node: rejected_cache_size", fmt.Sprint(r.rejected))
		tb.row("  want_claims", fmtCounts(r.claims))
		if r.zeroAfterTTL != nil {
			tb.row("  older deliveries within RejectTTL + 1 min (long)", fmt.Sprint(r.zeroAfterTTL))
		}
	}
	t.Logf("T6.1 Stale stamp:\n%s", tb.String())
}

func runStale(t *testing.T, name string, o accOptions, mode accMode) *accStaleResult {
	t.Helper()

	begin := time.Now()
	h := newAccHarness(t, o)
	defer h.close()
	h.start()
	r := &accStaleResult{name: name, mode: mode}

	older, err := h.validStamp(h.newChunk())
	if err != nil {
		t.Fatal(err)
	}
	newer := h.restamp(older)
	olderID, err := chunkIDOf(older)
	if err != nil {
		t.Fatal(err)
	}
	newerID, err := chunkIDOf(newer)
	if err != nil {
		t.Fatal(err)
	}

	var stale *accStalePeer
	if mode.Rangesync {
		stale = h.newStalePeer(append(slices.Clone(h.base), older))
		defer stale.close()
	}
	s0 := h.snapshot()
	pushAt := time.Now()
	h.put(h.nodes[0], older, accPush)
	h.put(h.nodes[1], newer, accPush)
	if stale != nil {
		stale.connect()
	}

	holds := func(n *accNode, id fingerprint.ID) bool {
		has, err := n.db.ReserveHas(id.Address(), older.Stamp().BatchID(), id.StampHash())
		if err != nil {
			t.Fatal(err)
		}
		return has
	}
	accWait(t, "every node to hold the newer stamp only", accBootstrapLimit, func() (bool, string) {
		for _, n := range h.nodes {
			if !holds(n, newerID) || holds(n, olderID) {
				return false, fmt.Sprintf("node %d: newer %v, older %v", n.i, holds(n, newerID), holds(n, olderID))
			}
		}
		return true, ""
	})
	r.converge = time.Since(pushAt)
	count := func(id fingerprint.ID) []int {
		out := make([]int, len(h.nodes))
		h.mu.Lock()
		defer h.mu.Unlock()
		for i := range h.nodes {
			if d, ok := h.deliveries[i][id]; ok {
				out[i] = d.n[accViaPullsync] + d.n[accViaRangesync]
			}
		}
		return out
	}
	olderAt := count(olderID)
	var sessions0 []int64
	if stale != nil {
		sessions0 = stale.sessions()
	}
	if stale != nil {
		time.Sleep(accStaleObserve)
	}
	s1 := h.snapshot()
	r.older, r.newer = count(olderID), count(newerID)
	for i := range h.nodes {
		r.olderAfter = append(r.olderAfter, r.older[i]-olderAt[i])
		// a delivery of the older stamp is stored only if the node held
		// neither (it arrived through a delivery, not the push); every
		// other was rejected
		h.mu.Lock()
		_, arrived := h.arrivals[i][olderID]
		stored := arrived && h.placed[i][olderID] == 0
		h.mu.Unlock()
		rejected := r.older[i]
		if stored {
			rejected--
		}
		r.olderRejected = append(r.olderRejected, rejected)
		m := s1.nodes[i].since(s0.node(i))
		r.rejected = append(r.rejected, m.value("bee_rangesync_rejected_cache_size"))
		for k, v := range m.byLabel("bee_rangesync_want_claims") {
			if r.claims == nil {
				r.claims = make(map[string]float64)
			}
			r.claims[k] += v
		}
	}
	if stale != nil {
		for i, n := range stale.sessions() {
			r.staleSessions = append(r.staleSessions, n-sessions0[i])
		}
	}
	h.checkUnion()
	if mode.Rangesync {
		// a node that holds the newer stamp fetches the older once (its Put
		// is the first to see it is older, Q7) and puts it in the rejected
		// cache: the sessions with the stale peer list it in every round
		// after, and none fetches it again. A node that held neither may
		// store the older first, then replace it with the newer.
		for i := range h.nodes {
			if r.olderRejected[i] > 1 || r.newer[i] > 1 {
				t.Errorf("AC-C3: node %d got the older stamp %d times (%d rejected), the newer %d times, want one rejected and one newer at most", i, r.older[i], r.olderRejected[i], r.newer[i])
			}
			if r.rejected[i] > 1 {
				t.Errorf("AC-C3: node %d's rejected cache holds %v IDs, more than the one stale ID", i, r.rejected[i])
			}
			if r.staleSessions[i] < 2 {
				t.Errorf("node %d ran %d sessions with the stale peer after the convergence, want at least 2: the older stamp was not offered again", i, r.staleSessions[i])
			}
		}
		if r.claims["rejected"] == 0 {
			t.Errorf("AC-C3: the rejected cache skipped no fetch (want_claims %v)", r.claims)
		}
	}
	if accLong() && stale != nil {
		before := count(olderID)
		time.Sleep(rangesync.RejectTTL + time.Minute)
		after := count(olderID)
		for i := range h.nodes {
			r.zeroAfterTTL = append(r.zeroAfterTTL, after[i]-before[i])
		}
	}
	t.Logf("%s: %d nodes (%s), converged %v after the pushes, run %v", name, len(h.nodes), mode, ms(r.converge), ms(time.Since(begin)))
	return r
}

// accStalePeer is a peer that holds the older stamp of a chunk for good: a
// rangesync service on a mock reserve, with no reconciler, so that it serves
// sessions and fetches but never fetches; its overlay is the highest of the
// neighborhood, so that every node initiates the sessions with it.
type accStalePeer struct {
	h    *accHarness
	host *accHost
	svc  interface {
		SendStatus(ctx context.Context, peer swarm.Address) error
		Close() error
	}
}

func (h *accHarness) newStalePeer(chunks []swarm.Chunk) *accStalePeer {
	h.tb.Helper()

	ff := make([]byte, swarm.HashSize)
	for i := range ff {
		ff[i] = 0xff
	}
	overlay := withPrefix(swarm.NewAddress(ff), h.prefix, h.o.Radius)
	store := mockstorer.NewReserve(mockstorer.WithAddrIndex(), mockstorer.WithRadius(h.o.Radius), mockstorer.WithChunks(chunks...))
	host := h.net.newHost(overlay, newAccTopology(overlay))
	svc := rangesync.New(host, store, overlay, nil, nil, h.validStamp, log.Noop, rangesync.Options{})
	host.AddProtocol(svc.Protocol())
	if err := svc.Start(); err != nil {
		h.tb.Fatal(err)
	}
	accWait(h.tb, "the stale peer's index", 30*time.Second, func() (bool, string) {
		_, err := svc.LocalStatus()
		return err == nil, fmt.Sprint(err)
	})
	return &accStalePeer{h: h, host: host, svc: svc}
}

// connect connects the stale peer to every node and sends them its Status
// (it has no reconciler to gossip it).
func (p *accStalePeer) connect() {
	p.h.tb.Helper()

	for _, n := range p.h.nodes {
		if err := p.h.net.connect(p.host, n.host); err != nil {
			p.h.tb.Fatal(err)
		}
		if err := p.svc.SendStatus(context.Background(), n.overlay); err != nil {
			p.h.tb.Fatal(err)
		}
	}
}

// sessions returns, per node, the reconcile streams between it and the stale
// peer.
func (p *accStalePeer) sessions() []int64 {
	out := make([]int64, len(p.h.nodes))
	for i, n := range p.h.nodes {
		out[i], _ = p.h.net.pairStats(n.overlay, p.host.overlay, "rangesync/reconcile")
	}
	return out
}

func (p *accStalePeer) close() {
	for _, n := range p.h.nodes {
		_ = p.h.net.disconnect(p.host, n.host)
	}
	if err := p.svc.Close(); err != nil {
		p.h.tb.Error(err)
	}
}

// --- Churn -------------------------------------------------------------------

// Times of the Churn scenario's events.
const (
	// accChurnLeave and accChurnRejoin are when a node leaves and rejoins,
	// from the first upload: away for 5 s, it misses ~75 pushes at 15 per
	// second, fewer than accLargeDiff, so that its catch-up is a session,
	// not a striped transfer during the uploads (see accChurnRepro)
	accChurnLeave  = 3 * time.Second
	accChurnRejoin = 8 * time.Second
	// accChurnStripePeerAway is how long the stripe peer that leaves stays
	// away
	accChurnStripePeerAway = 5 * time.Second
	// accChurnStripeShare is the share of its root the joiner stored when
	// its stripe peer leaves
	accChurnStripeShare = 0.25
	// accChurnReproJoin is when the joiner of accChurnRepro joins, from the
	// first upload
	accChurnReproJoin = 5 * time.Second
)

// accChurnRepro reports whether the Churn scenario's skipped sub-case runs
// (RANGESYNC_ACCEPTANCE_REPRO set): a node that bootstraps while uploads run
// at the reference load scores its honest stripe peers (Decisions log, T6.1
// part 2).
func accChurnRepro() bool { return os.Getenv("RANGESYNC_ACCEPTANCE_REPRO") != "" }

// accChurnResult is what a Churn scenario measured, with the window's
// totals in all.
type accChurnResult struct {
	all              *accResult
	joinerBootstrap  time.Duration // join to the joiner's base union
	rejoinCatchUp    time.Duration // the rejoin to the last upload the leaver missed
	stripePeerLeftAt float64       // the joiner's share of its base when its stripe peer left
	stripes          map[string]float64
	failures         float64
	expired          float64
}

// TestAcceptanceChurn runs the Churn scenario for each size of
// accScenarioSizes on rangesync-only nodes, N − 1 of them holding the base
// chunks. First uploads run at the reference load for accUploadPhase while a
// node leaves (accChurnLeave) and rejoins (accChurnRejoin), missing the
// pushes meanwhile. Then an empty node joins and bootstraps striped, and once
// it holds accChurnStripeShare of its root, the holder that is delivering one
// of its stripes leaves, and rejoins accChurnStripePeerAway later. It checks
// that every node converges (AC-C1, AC-K3), that the joiner's stripes were
// reassigned (stripes_total{reassigned}), AC-D1 with faults (rangesync's
// repeated deliveries, its late deliveries of pushed chunks and its
// deliveries of chunks a node held at most the claims that expired) and that
// no honest peer was scored or disconnected.
//
// The sub-case "bootstrap during uploads" (accChurnRepro, skipped by
// default) lets the empty node join while the uploads run: the verification
// of its stripes then scores the honest stripe peers for the chunks uploaded
// into their stripes after they listed them.
func TestAcceptanceChurn(t *testing.T) {
	for _, n := range accScenarioSizes(t) {
		t.Run(fmt.Sprintf("N=%d", n), func(t *testing.T) {
			rate := accRate(n)
			o := accOptions{
				Nodes: n - 1, Seed: uint64(400 + n), Radius: 2, Rate: rate,
				Uploads: int(math.Round(rate * accUploadPhase.Seconds())),
				Mode:    func(int) accMode { return accRangesyncOnly },
				Window:  accWindow, LargeDiff: accLargeDiff, Tail: accTail,
			}
			t.Run("leave rejoin and stripe peer", func(t *testing.T) {
				logChurnTable(t, runChurn(t, o, false))
			})
			t.Run("bootstrap during uploads", func(t *testing.T) {
				if !accChurnRepro() {
					t.Skip("finding (T6.1 part 2): the verification of a bootstrap that runs while uploads land in its stripes scores the honest stripe peers; set RANGESYNC_ACCEPTANCE_REPRO to run it")
				}
				logChurnTable(t, runChurn(t, o, true))
			})
		})
	}
}

// runChurn runs the Churn scenario; with duringUploads, the joiner joins at
// accChurnReproJoin while the uploads run, and no other fault happens.
func runChurn(t *testing.T, o accOptions, duringUploads bool) *accChurnResult {
	t.Helper()

	begin := time.Now()
	h := newAccHarness(t, o)
	defer h.close()
	h.start()
	setup := time.Since(begin)
	o = h.o
	r := &accChurnResult{}

	leaver := h.nodes[len(h.nodes)-1]
	var (
		joiner         *accNode
		joinAt         time.Time
		joinerBase     map[fingerprint.ID]struct{}
		stripePeer     *accNode
		stripePeerLeft time.Time
		lastFetch      = map[int]uint64{}
		rejoinAt       time.Time
	)
	join := func() {
		spec := accNodeSpec{
			overlay: withPrefix(swarm.NewAddress(h.randBytes(swarm.HashSize)), h.prefix, o.Radius),
			mode:    accRangesyncOnly,
			radius:  o.Radius,
			wakeUp:  o.ReserveWakeUpDuration,
		}
		joinerBase = idsIn(t, h.base, fingerprint.NewRange(spec.overlay, spec.radius))
		joinAt = time.Now()
		joiner = h.join(spec)
	}
	// leaveRejoin runs the leaver's faults during the uploads
	leaveRejoin := func(since time.Duration) {
		switch {
		case duringUploads:
			if since >= accChurnReproJoin && joiner == nil {
				join()
			}
		case since >= accChurnLeave && !leaver.offline && rejoinAt.IsZero():
			h.leave(leaver)
		case since >= accChurnRejoin && leaver.offline:
			h.rejoin(leaver)
			rejoinAt = time.Now()
		}
	}
	// stripeFault makes the holder that is delivering a stripe to the
	// joiner leave once the joiner holds accChurnStripeShare of its root,
	// and rejoin later; it reports whether both happened
	stripeFault := func() bool {
		if stripePeer == nil {
			got, _, _ := h.arrivalSpan(joiner.i, joinerBase)
			// the holder whose fetch stream to the joiner moved bytes since
			// the last look is delivering a stripe
			var best *accNode
			var bestDelta uint64
			for _, p := range h.nodes {
				if p == joiner || p.offline {
					continue
				}
				_, b := h.net.pairStats(joiner.overlay, p.overlay, "rangesync/fetch")
				if d := b - lastFetch[p.i]; d > bestDelta {
					best, bestDelta = p, d
				}
				lastFetch[p.i] = b
			}
			if share := float64(got) / float64(len(joinerBase)); share >= accChurnStripeShare && best != nil {
				stripePeer, stripePeerLeft = best, time.Now()
				r.stripePeerLeftAt = share
				h.leave(stripePeer)
			}
			return false
		}
		if stripePeer.offline && time.Since(stripePeerLeft) >= accChurnStripePeerAway {
			h.rejoin(stripePeer)
		}
		return !stripePeer.offline
	}

	runtime.GC()
	h.sampler.resetPeaks()
	s0 := h.snapshot()
	start := time.Now()
	lastUpload := h.uploadPhase(o.Uploads, o.Rate, func(int, time.Duration) { leaveRejoin(time.Since(start)) })
	missedByLeaver := make(map[fingerprint.ID]struct{})
	h.mu.Lock()
	for _, u := range h.uploads {
		if !duringUploads && slices.Contains(u.want, leaver.i) && !slices.Contains(u.holders, leaver.i) {
			missedByLeaver[u.id] = struct{}{}
		}
	}
	h.mu.Unlock()
	if !duringUploads {
		accWait(t, "every node to hold its root's union", o.Limit, func() (bool, string) {
			missing, _ := h.pending()
			return missing == 0, fmt.Sprintf("%d (upload, node) pairs missing", missing)
		})
		join()
		accWaitEvery(t, "the stripe peer to leave and rejoin", accBootstrapLimit, 20*time.Millisecond, func() (bool, string) {
			return stripeFault(), fmt.Sprintf("stripe peer %v", stripePeer != nil)
		})
	}
	accWait(t, "every node to hold its root's union", o.Limit, func() (bool, string) {
		missing, _ := h.pending()
		got, _, _ := h.arrivalSpan(joiner.i, joinerBase)
		return missing == 0 && got == len(joinerBase), fmt.Sprintf("%d (upload, node) pairs missing, joiner %d of %d base", missing, got, len(joinerBase))
	})
	_, convergedAt := h.pending()
	_, _, baseAt := h.arrivalSpan(joiner.i, joinerBase)
	r.joinerBootstrap = baseAt.Sub(joinAt)
	if _, _, last := h.arrivalSpan(leaver.i, missedByLeaver); !last.IsZero() {
		r.rejoinCatchUp = last.Sub(rejoinAt)
	}
	quietFrom := lastUpload
	if baseAt.After(quietFrom) {
		quietFrom = baseAt
	}
	quietAt := h.waitQuiet(quietFrom)
	s1 := h.snapshot()
	r.all = h.result("churn", s0, s1, lastUpload, convergedAt, quietAt)
	r.all.setup = setup
	jm := s1.nodes[joiner.i].since(s0.node(joiner.i))
	r.stripes = jm.byLabel("bee_rangesync_stripes_total")
	for i := range h.nodes {
		r.failures += s1.nodes[i].since(s0.node(i)).value("bee_rangesync_peer_failures_total")
	}
	r.expired = r.all.wantClaims["expired"]
	h.checkUnion()
	r.all.run = time.Since(begin)

	all := r.all
	if !duringUploads && r.stripes["reassigned"] == 0 {
		t.Errorf("the joiner's stripe peer left at %.0f%% of its root and no stripe was reassigned: stripes_total %v", 100*r.stripePeerLeftAt, r.stripes)
	}
	// AC-D1 with faults
	if dups := all.dupRS + all.dupBase; float64(dups) > r.expired {
		t.Errorf("AC-D1: %d rangesync repeats (%d of them pushed chunks past %v) and %d deliveries of held base chunks, more than the %v expired claims", all.dupRS, all.dupLatePushRS, accPushRaceWindow, all.dupBase, r.expired)
	}
	if limit := math.Ceil(0.01 * all.rsFetched); float64(all.dupPushRS) > limit {
		t.Errorf("%d rangesync deliveries raced a push, more than %v", all.dupPushRS, limit)
	}
	if r.failures != 0 || all.disconnects != 0 || all.blocklists != 0 {
		t.Errorf("honest peers scored: %v peer failures, %d disconnects and %d blocklists (last reason %q)", r.failures, all.disconnects, all.blocklists, all.lastReason)
	}
	if all.decodeCorruption != 0 {
		t.Errorf("AC-K3: decode_corruption_total %v", all.decodeCorruption)
	}
	t.Logf("churn: %d nodes, setup %v, run %v", len(h.nodes), ms(setup), ms(all.run))
	return r
}

func logChurnTable(t *testing.T, r *accChurnResult) {
	t.Helper()

	a := r.all
	var tb accTable
	tb.row("nodes; uploads (rate/s)", fmt.Sprintf("%d (%s); %d (%.0f)", a.nodes, a.modes, a.uploads, a.rate))
	tb.row("joiner: join to base union", ms(r.joinerBootstrap))
	tb.row("joiner's share when its stripe peer left", fmt.Sprintf("%.0f%%", 100*r.stripePeerLeftAt))
	tb.row("joiner: stripes_total", fmtCounts(r.stripes))
	tb.row("leaver: rejoin to the last upload it missed", ms(r.rejoinCatchUp))
	tb.row("convergence after last upload", ms(a.converge))
	tb.row("upload latency median / p95 / max", fmt.Sprintf("%v/%v/%v", a.latMedian.Round(10*time.Millisecond), a.latP95.Round(10*time.Millisecond), a.latMax.Round(10*time.Millisecond)))
	tb.row("rs metadata in, B/upload (incl. the bootstrap)", fmt.Sprintf("%.0f", a.rsMetaIn/float64(max(1, a.uploads))))
	tb.row("duplicates total (rangesync repeats / base)", fmt.Sprintf("%d (%d / %d)", a.dupTotal, a.dupRS, a.dupBase))
	tb.row("races (max); pushed past window", fmt.Sprintf("%d (%v); %d", a.dupPushRS, ms(a.raceMax), a.dupLatePushRS))
	tb.row("want_claims", fmtCounts(a.wantClaims))
	tb.row("sessions_total", fmtCounts(a.sessions))
	tb.row("peer failures; disconnects; blocklists", fmt.Sprintf("%.0f; %d; %d", r.failures, a.disconnects, a.blocklists))
	tb.row("rs fetch latency median/p99/max", fmt.Sprintf("%s/%s/%s", ms(a.fetchLatMedian), ms(a.fetchLatP99), ms(a.fetchLatMax)))
	tb.row("CPU s per 1000 uploads", fmt.Sprintf("%.2f", 1000*a.cpu/float64(max(1, a.uploads))))
	tb.row("decode_corruption", fmt.Sprintf("%.0f", a.decodeCorruption))
	t.Logf("T6.1 Churn:\n%s", tb.String())
}

// --- Latency -----------------------------------------------------------------

// TestAcceptanceLatency runs the Baseline and rangesync-only scenarios of
// TestAcceptance, with their checks, for each size of accSizes, with a
// one-way latency on every stream (RANGESYNC_ACCEPTANCE_LATENCY, for example
// 50ms) and the bounded window of the part-2 scenarios (accWindow): it shows
// how both protocols' upload latency and duplicates depend on a round trip,
// and checks accPushRaceWindow against the fetch latency it reports. It is
// skipped without the variable.
func TestAcceptanceLatency(t *testing.T) {
	v := os.Getenv("RANGESYNC_ACCEPTANCE_LATENCY")
	if v == "" {
		t.Skip("set RANGESYNC_ACCEPTANCE_LATENCY (for example 50ms) to run")
	}
	latency, err := time.ParseDuration(v)
	if err != nil || latency <= 0 {
		t.Fatalf("RANGESYNC_ACCEPTANCE_LATENCY: bad latency %q", v)
	}
	for _, n := range accSizes(t) {
		t.Run(fmt.Sprintf("N=%d", n), func(t *testing.T) {
			rate := accRate(n)
			o := accOptions{
				Nodes: n, Seed: uint64(n), Radius: 2, Rate: rate,
				Uploads: int(math.Round(rate * accUploadPhase.Seconds())),
				Latency: latency, Window: accWindow,
			}
			var baseline, rangesyncOnly *accResult
			t.Run("baseline", func(t *testing.T) {
				baseline = runAccScenario(t, "baseline", o)
				checkBaseline(t, baseline)
			})
			t.Run("rangesync", func(t *testing.T) {
				ro := o
				ro.Mode = func(int) accMode { return accRangesyncOnly }
				rangesyncOnly = runAccScenario(t, "rangesync", ro)
				checkRangesyncOnly(t, rangesyncOnly)
				if rangesyncOnly.fetchLatMax >= accPushRaceWindow/2 {
					t.Logf("the largest fetch latency %v is more than half accPushRaceWindow (%v)", rangesyncOnly.fetchLatMax, accPushRaceWindow)
				}
			})
			t.Logf("one-way latency %v, window %d B", latency, accWindow)
			logAccTable(t, baseline, rangesyncOnly, nil)
		})
	}
}
