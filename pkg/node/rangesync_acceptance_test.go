// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build integration

package node_test

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// The scenarios of plan task T6.1 (part 1): Baseline (every node
// pullsync-only), rangesync only (every node rangesync-only) and Mixed
// (pullsync-only, rangesync-only and dual nodes), each at the reference load
// of acceptance-criteria.md §1, on the same overlays and uploads for one
// network size, then compared with Baseline.

// accSizes returns the network sizes to run: RANGESYNC_ACCEPTANCE_SIZES (a
// comma-separated list), 5 and 9 by default.
func accSizes(t *testing.T) []int {
	t.Helper()

	v := os.Getenv("RANGESYNC_ACCEPTANCE_SIZES")
	if v == "" {
		return []int{5, 9}
	}
	var sizes []int
	for _, f := range strings.Split(v, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil || n < 3 {
			t.Fatalf("RANGESYNC_ACCEPTANCE_SIZES: bad size %q", f)
		}
		sizes = append(sizes, n)
	}
	return sizes
}

// accUploadPhase is how long the uploads last: 4 rounds of MinRoundInterval.
const accUploadPhase = 20 * time.Second

// accRate returns the upload rate of a network of n nodes, at the reference
// load of acceptance-criteria.md §1 (median pairwise_diff ≥ 20 per
// steady-state session): 15 per second up to 9 nodes, the simulation's
// steadyOptions rate. Two nodes differ on the uploads one of them holds and
// the other lacks, a share 2p(N − p)/(N(N − 1)) of them with p holders, so
// at a fixed rate a pair's difference per round falls with N: 37.5 per
// 5 s at N = 9, 23 at N = 17, whose median then fell in the pairwise_diff
// bucket 16–31, which cannot tell 16–19 from 20–31. Above 9 nodes the rate
// gives a pair 1.2 × N = 9's expected difference per 5 s round (45), rounded
// up to 5 per second: 30 at N = 17.
func accRate(n int) float64 {
	const base = 15.0
	if n <= 9 {
		return base
	}
	const p = 3.0 // accOptions.Replication's default
	share := func(n float64) float64 { return 2 * p * (n - p) / (n * (n - 1)) }
	return 5 * math.Ceil(1.2*base*share(9)/share(float64(n))/5)
}

// accMixedMode gives the nodes of the Mixed scenario their flags in turn:
// pullsync-only, rangesync-only, dual.
func accMixedMode(i int) accMode {
	return [...]accMode{accPullsyncOnly, accRangesyncOnly, accDual}[i%3]
}

// TestAcceptance runs the Baseline, rangesync-only and Mixed scenarios for
// each network size (accSizes) and logs their metrics side by side:
//   - Baseline (every node pullsync-only): every node ends with its
//     neighborhood's union; its numbers are the reference;
//   - rangesync only: every node ends with its neighborhood's union and its
//     Index equals a brute force over its reserve (AC-C1); rangesync
//     delivers every ID at most once per node and no chunk the node held at
//     the start (AC-D1), and at most 1 % of its deliveries race the chunk's
//     push (AC-D2's draft bound), each within accPushRaceWindow of it;
//     every upload reaches every node within 2 × MinRoundInterval (AC-V1);
//     no corrupt decode (AC-K3) and quiescence within 120 s of the last
//     upload (AC-C2);
//   - Mixed: every node converges (AC-C1); on every rangesync-only and dual
//     node rangesync meets AC-D1 as above, and the pullsync-only nodes pull
//     from it (AC-S3); AC-V1 and AC-K3.
//
// The upload rate is accRate(N), the reference load.
func TestAcceptance(t *testing.T) {
	for _, n := range accSizes(t) {
		t.Run(fmt.Sprintf("N=%d", n), func(t *testing.T) {
			seed := uint64(n)
			rate := accRate(n)
			o := accOptions{Nodes: n, Seed: seed, Radius: 2, Rate: rate, Uploads: int(math.Round(rate * accUploadPhase.Seconds()))}
			var baseline, rangesyncOnly, mixed *accResult
			t.Run("baseline", func(t *testing.T) {
				baseline = runAccScenario(t, "baseline", o)
				checkBaseline(t, baseline)
			})
			t.Run("rangesync", func(t *testing.T) {
				ro := o
				ro.Mode = func(int) accMode { return accRangesyncOnly }
				rangesyncOnly = runAccScenario(t, "rangesync", ro)
				checkRangesyncOnly(t, rangesyncOnly)
			})
			t.Run("mixed", func(t *testing.T) {
				mo := o
				mo.Mode = accMixedMode
				mixed = runAccScenario(t, "mixed", mo)
				checkMixed(t, mixed)
			})
			logAccTable(t, baseline, rangesyncOnly, mixed)
		})
	}
}

// TestAccDuplicateClassification checks the suite's duplicate
// classification (classifyDeliveries): rangesync's first delivery of a pushed
// chunk races the push if it lands within accPushRaceWindow of it, before or
// after; past it (a late refetch) it is a rangesync repeat, as a second
// delivery is; a delivery of a base chunk is a false difference, a fetched ID
// delivered by both protocols a cross-protocol duplicate, and pullsync's
// first delivery of a pushed chunk is not timed.
func TestAccDuplicateClassification(t *testing.T) {
	t.Parallel()

	push := time.Now()
	id := func(b byte) fingerprint.ID {
		var x fingerprint.ID
		x[0] = b
		return x
	}
	deliveries := map[fingerprint.ID]*accDeliveries{
		id(1): {n: [2]int{0, 1}, first: [2]time.Time{{}, push.Add(100 * time.Millisecond)}},
		id(2): {n: [2]int{0, 1}, first: [2]time.Time{{}, push.Add(-50 * time.Millisecond)}},
		id(3): {n: [2]int{0, 1}, first: [2]time.Time{{}, push.Add(accPushRaceWindow + time.Millisecond)}},
		id(4): {n: [2]int{0, 2}},
		id(5): {n: [2]int{0, 1}},
		id(6): {n: [2]int{1, 1}},
		id(7): {n: [2]int{1, 0}, first: [2]time.Time{push.Add(3 * accPushRaceWindow)}},
	}
	placed := map[fingerprint.ID]accPlacement{id(1): accPush, id(2): accPush, id(3): accPush, id(5): accBase, id(7): accPush}
	pushed := map[fingerprint.ID]time.Time{id(1): push, id(2): push, id(3): push, id(7): push}

	got := classifyDeliveries(deliveries, placed, pushed, accPushRaceWindow)
	want := accDups{total: 7, base: 1, pushPS: 1, pushRS: 2, latePushRS: 1, rs: 2, cross: 1, raceMax: 100 * time.Millisecond}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func checkBaseline(t *testing.T, r *accResult) {
	t.Helper()

	if r.psOffered == 0 || r.psDelivered == 0 {
		t.Errorf("pullsync offered %v and delivered %v chunks", r.psOffered, r.psDelivered)
	}
	if r.wire["rangesync"] != 0 || r.rsMetaIn != 0 {
		t.Errorf("rangesync traffic in a pullsync-only network: %d wire bytes", r.wire["rangesync"])
	}
	checkNoFaults(t, r)
}

func checkRangesyncOnly(t *testing.T, r *accResult) {
	t.Helper()

	// AC-D1: rangesync delivers every ID at most once per node, and no base
	// chunk; a delivery that raced the chunk's push (pushsync racing a
	// fetch, AC-D2) is reported, not a rangesync duplicate, if it landed
	// within accPushRaceWindow of the push
	if r.dupRS != 0 || r.dupBase != 0 {
		t.Errorf("rangesync delivered %d IDs again (%d of them pushed chunks past %v of the push) and %d base chunks, want 0", r.dupRS, r.dupLatePushRS, accPushRaceWindow, r.dupBase)
	}
	// AC-D2's draft bound on those races, 1 % of the chunks fetched
	if limit := math.Ceil(0.01 * r.rsFetched); float64(r.dupPushRS) > limit {
		t.Errorf("%d rangesync deliveries raced a push, more than %v", r.dupPushRS, limit)
	}
	if r.wire["pullsync"] != 0 || r.psDelivered != 0 {
		t.Errorf("pullsync traffic without a puller: %d wire bytes, %v deliveries", r.wire["pullsync"], r.psDelivered)
	}
	if r.decodeCorruption != 0 {
		t.Errorf("decode_corruption_total %v, want 0", r.decodeCorruption)
	}
	if r.rsFetched == 0 {
		t.Error("rangesync fetched nothing")
	}
	checkConvergenceTime(t, r)
	checkNoFaults(t, r)
}

func checkMixed(t *testing.T, r *accResult) {
	t.Helper()

	for i, nr := range r.perNode {
		if !nr.mode.Rangesync {
			continue
		}
		// AC-D1 on the rangesync drivers of the rangesync-only and dual
		// nodes, where the index and the observer run next to pullsync
		if nr.dupRS != 0 || nr.dupBase != 0 {
			t.Errorf("node %d (%s): rangesync delivered %d IDs again and the node got %d base chunks, want 0", i, nr.mode, nr.dupRS, nr.dupBase)
		}
		// AC-S3: the pullsync-only nodes pull from it (from a dual node,
		// the T1.13 gap)
		if nr.psSentOffered == 0 || nr.psSent == 0 {
			t.Errorf("node %d (%s) served pullsync peers %v offers and %v chunks, want both > 0", i, nr.mode, nr.psSentOffered, nr.psSent)
		}
		if nr.mode == accRangesyncOnly && nr.psDelivered != 0 {
			t.Errorf("rangesync-only node %d pulled %v chunks with pullsync", i, nr.psDelivered)
		}
	}
	if r.decodeCorruption != 0 {
		t.Errorf("decode_corruption_total %v, want 0", r.decodeCorruption)
	}
	checkConvergenceTime(t, r)
	checkNoFaults(t, r)
}

// checkConvergenceTime checks AC-V1: every upload reached every node whose
// root holds it within 2 × MinRoundInterval, so within 2 × RoundInterval
// whatever the adaptive interval was at the upload.
func checkConvergenceTime(t *testing.T, r *accResult) {
	t.Helper()

	if r.v1Within != r.uploads {
		t.Errorf("AC-V1: %d of %d uploads reached every node within %v (the slowest took %v)", r.v1Within, r.uploads, 2*reconciler.MinRoundInterval, r.latMax)
	}
}

// checkNoFaults checks that no protocol disconnected or blocklisted a peer:
// the scenarios have no faults.
func checkNoFaults(t *testing.T, r *accResult) {
	t.Helper()

	if r.disconnects != 0 || r.blocklists != 0 {
		t.Errorf("%d disconnects and %d blocklists (last reason %q), want none", r.disconnects, r.blocklists, r.lastReason)
	}
}

// accResult is what a scenario measured, summed over the nodes, from the
// first upload to the end of the quiet tail (the window).
type accResult struct {
	name    string
	nodes   int
	modes   string
	uploads int
	rate    float64
	window  time.Duration

	// convergence: from the last upload until every node's root holds every
	// upload (AC-C1's union condition), and per upload until it is on every
	// node whose root holds it
	converge                    time.Duration
	latMedian, latP95, latMax   time.Duration
	v1Within                    int     // uploads on every node within 2 × MinRoundInterval (AC-V1)
	roundMin, roundMax          float64 // round interval range sampled in the window, s
	quiet                       time.Duration
	setup, run                  time.Duration
	cpu                         float64 // process CPU seconds in the window
	heapPeak                    uint64
	indexPeakNode, indexPeakSum float64
	indexPeakRun                float64 // largest of a node over the whole run

	rsMetaIn, rsMetaOut, rsChunksIn, rsChunksOut float64
	rsFetched                                    float64
	psOffered, psWanted, psDelivered, psSent     float64
	wire                                         map[string]uint64 // bytes written, by protocol
	wireStream                                   map[string]uint64 // bytes written, by protocol/stream
	psDeliveryWire                               uint64
	// duplicate deliveries (classifyDeliveries): of base chunks (false
	// differences), racing a push (by protocol), a protocol's repeats
	// (rangesync's with its late deliveries of pushed chunks),
	// cross-protocol
	dupTotal, dupBase, dupPushPS, dupPushRS int
	dupLatePushRS                           int
	dupPS, dupRS, dupCross                  int
	raceMax                                 time.Duration // the largest distance of a rangesync race from its push
	crossMetric                             float64
	// the window's bounds (accPushRaceWindow): the rangesync fetch latency
	// from the reserve's last miss, and the longest push over its holders
	fetchLatMedian, fetchLatP99, fetchLatMax time.Duration
	pushSpan                                 time.Duration
	wantClaims, sessions, parts              map[string]float64
	wantsDecoded                             float64 // Σ pairwise_diff_half: IDs sessions found missing
	symMedian, symP95, symMean               float64
	symCount                                 uint64
	diffMedian, diffHalfMedian, diffHalfP99  float64
	diffCount                                uint64
	decodeCorruption, moreRequests           float64
	disconnects, blocklists                  int64
	lastReason                               string
	perNode                                  []accNodeResult
}

// accNodeResult is what one node measured.
type accNodeResult struct {
	mode                               accMode
	psSent, psSentOffered, psDelivered float64
	rsFetched, cross                   float64
	// duplicate deliveries, deliveries of base chunks, rangesync's repeats
	// (with its late deliveries of pushed chunks) and rangesync's
	// deliveries racing a push (classifyDeliveries)
	dups, dupBase, dupRS, pushRS int
}

// runAccScenario builds the network of o, waits until it is ready, pushes
// the uploads at the reference rate, waits until every node holds its
// root's union and the network is quiet, and returns what it measured. It
// then checks AC-C1 (checkUnion) and shuts the network down.
func runAccScenario(t *testing.T, name string, o accOptions) *accResult {
	t.Helper()

	begin := time.Now()
	h := newAccHarness(t, o)
	defer h.close()
	o = h.o
	h.start()
	setup := time.Since(begin)

	chunks := make([]swarm.Chunk, o.Uploads)
	for i := range chunks {
		chunks[i] = h.newChunk()
	}

	// the garbage of the setup (and of an earlier scenario) is not the
	// window's
	runtime.GC()
	h.sampler.resetPeaks()
	s0 := h.snapshot()
	interval := time.Duration(float64(time.Second) / o.Rate)
	next := time.Now()
	for _, ch := range chunks {
		time.Sleep(time.Until(next))
		h.push(ch)
		next = next.Add(interval)
	}
	lastUpload := time.Now()

	accWait(t, "every node to hold its root's union", o.Limit, func() (bool, string) {
		missing, _ := h.pending()
		return missing == 0, fmt.Sprintf("%d (upload, node) pairs missing", missing)
	})
	_, convergedAt := h.pending()
	quietAt := h.waitQuiet(lastUpload)
	s1 := h.snapshot()

	r := h.result(name, s0, s1, lastUpload, convergedAt, quietAt)
	r.setup = setup
	h.checkUnion()
	r.run = time.Since(begin)
	t.Logf("%s: %d nodes (%s), setup %v, run %v", name, r.nodes, r.modes, r.setup.Round(time.Millisecond), r.run.Round(time.Millisecond))
	return r
}

// waitQuiet waits until the network is quiet: every rangesync node's
// connected rangesync peers hold its current Status, nodes with the same
// root have the same digest, and no reconciler started a session for the
// tail. It returns when the last session started, and fails after the limit
// from the last upload (AC-C2). Without rangesync nodes it waits the tail.
func (h *accHarness) waitQuiet(lastUpload time.Time) time.Time {
	h.tb.Helper()

	if !slices.ContainsFunc(h.nodes, func(n *accNode) bool { return n.svc != nil }) {
		time.Sleep(h.o.Tail)
		return lastUpload
	}
	deadline := lastUpload.Add(h.o.Limit)
	for {
		accWait(h.tb, "equal digests", time.Until(deadline), func() (bool, string) {
			return h.statusesCurrent()
		})
		count, _ := h.sampler.lastSession()
		time.Sleep(h.o.Tail)
		after, at := h.sampler.lastSession()
		if ok, _ := h.statusesCurrent(); ok && after == count {
			return at
		}
		if time.Now().After(deadline) {
			h.tb.Fatalf("sessions still start %v after the last upload", time.Since(lastUpload))
		}
	}
}

// statusesCurrent reports whether every rangesync node's connected
// rangesync peers hold its current Status and nodes with the same root have
// the same digest.
func (h *accHarness) statusesCurrent() (bool, string) {
	for _, n := range h.nodes {
		if n.svc == nil {
			continue
		}
		st, err := n.svc.LocalStatus()
		if err != nil {
			return false, fmt.Sprintf("node %d: %v", n.i, err)
		}
		for _, p := range h.nodes {
			if p == n || p.svc == nil || h.net.conn(n.overlay, p.overlay) == nil {
				continue
			}
			got, ok := p.svc.LastStatus(n.overlay)
			if !ok || !got.Root.Equal(st.Root) || got.Summary != st.Summary {
				return false, fmt.Sprintf("node %d's status at node %d is not current", n.i, p.i)
			}
			pst, err := p.svc.LocalStatus()
			if err != nil {
				return false, fmt.Sprintf("node %d: %v", p.i, err)
			}
			if pst.Root.Equal(st.Root) && pst.Summary != st.Summary {
				return false, fmt.Sprintf("nodes %d and %d: digests differ", n.i, p.i)
			}
		}
	}
	return true, ""
}

// result computes the scenario's result from the snapshots at the start and
// the end of the window.
func (h *accHarness) result(name string, s0, s1 accSnapshot, lastUpload, convergedAt, quietAt time.Time) *accResult {
	r := &accResult{
		name:        name,
		nodes:       len(h.nodes),
		uploads:     len(h.uploads),
		rate:        h.o.Rate,
		window:      s1.at.Sub(s0.at),
		cpu:         s1.process.value("bee_process_cpu_seconds_total") - s0.process.value("bee_process_cpu_seconds_total"),
		wire:        make(map[string]uint64),
		wireStream:  make(map[string]uint64),
		wantClaims:  make(map[string]float64),
		sessions:    make(map[string]float64),
		parts:       make(map[string]float64),
		disconnects: h.net.disconnects.Load(),
		blocklists:  h.net.blocklists.Load(),
	}
	if v, ok := h.net.lastReason.Load().(string); ok {
		r.lastReason = v
	}
	if convergedAt.After(lastUpload) {
		r.converge = convergedAt.Sub(lastUpload)
	}
	if quietAt.After(lastUpload) {
		r.quiet = quietAt.Sub(lastUpload)
	}
	var modes []string
	count := map[accMode]int{}
	for _, n := range h.nodes {
		count[n.mode]++
	}
	for _, m := range []accMode{accPullsyncOnly, accRangesyncOnly, accDual} {
		if count[m] > 0 {
			modes = append(modes, fmt.Sprintf("%d%s", count[m], m))
		}
	}
	r.modes = strings.Join(modes, "+")

	symbols := make([]accHist, 0, len(h.nodes))
	diff := make([]accHist, 0, len(h.nodes))
	diffHalf := make([]accHist, 0, len(h.nodes))
	for i, n := range h.nodes {
		m := s1.nodes[i].since(s0.nodes[i])
		nr := accNodeResult{
			mode:          n.mode,
			psSent:        m.value("bee_pullsync_chunks_sent"),
			psSentOffered: m.value("bee_pullsync_chunks_sent_offered"),
			psDelivered:   m.value("bee_pullsync_chunks_delivered"),
			rsFetched:     m.value("bee_rangesync_fetch_delivered_total"),
			cross:         m.value("bee_rangesync_cross_protocol_duplicates_total"),
		}
		r.psOffered += m.value("bee_pullsync_chunks_offered")
		r.psWanted += m.value("bee_pullsync_chunks_wanted")
		r.psDelivered += nr.psDelivered
		r.psSent += nr.psSent
		r.rsMetaIn += m.value(`bee_rangesync_bytes_metadata{dir="in"}`)
		r.rsMetaOut += m.value(`bee_rangesync_bytes_metadata{dir="out"}`)
		r.rsChunksIn += m.value(`bee_rangesync_bytes_chunks{dir="in"}`)
		r.rsChunksOut += m.value(`bee_rangesync_bytes_chunks{dir="out"}`)
		r.rsFetched += nr.rsFetched
		r.crossMetric += nr.cross
		r.decodeCorruption += m.value("bee_rangesync_decode_corruption_total")
		r.moreRequests += m.value("bee_rangesync_more_requests_total")
		for k, v := range m.byLabel("bee_rangesync_want_claims") {
			r.wantClaims[k] += v
		}
		for k, v := range m.byLabel("bee_rangesync_sessions_total") {
			r.sessions[k] += v
		}
		for k, v := range m.byLabel("bee_rangesync_parts_sent") {
			r.parts[k] += v
		}
		symbols = append(symbols, m.hists["bee_rangesync_symbols_per_difference"])
		diff = append(diff, m.hists["bee_rangesync_pairwise_diff"])
		diffHalf = append(diffHalf, m.hists["bee_rangesync_pairwise_diff_half"])
		for k, v := range s1.wire[i] {
			r.wire[strings.SplitN(k, "/", 2)[0]] += v - s0.wire[i][k]
			r.wireStream[k] += v - s0.wire[i][k]
		}
		r.perNode = append(r.perNode, nr)
	}
	r.psDeliveryWire = s1.psBytes - s0.psBytes
	sym := mergeHists(symbols...)
	r.symMedian, r.symP95, r.symCount = sym.quantile(0.5), sym.quantile(0.95), sym.count
	if sym.count > 0 {
		r.symMean = sym.sum / float64(sym.count)
	}
	d := mergeHists(diff...)
	r.diffMedian, r.diffCount = d.quantile(0.5), d.count
	dh := mergeHists(diffHalf...)
	r.diffHalfMedian, r.diffHalfP99 = dh.quantile(0.5), dh.quantile(0.99)
	r.wantsDecoded = dh.sum

	h.sampler.mu.Lock()
	r.heapPeak = h.sampler.heapPeak
	r.roundMin, r.roundMax = h.sampler.roundMin, h.sampler.roundMax
	for i, p := range h.sampler.indexPeak {
		r.indexPeakSum += p
		r.indexPeakNode = max(r.indexPeakNode, p)
		r.indexPeakRun = max(r.indexPeakRun, h.sampler.indexPeakRun[i])
	}
	h.sampler.mu.Unlock()

	h.mu.Lock()
	defer h.mu.Unlock()
	lat := make([]time.Duration, 0, len(h.uploads))
	for _, u := range h.uploads {
		var last time.Time
		for _, i := range u.want {
			if at, ok := h.arrivals[i][u.id]; ok && at.After(last) {
				last = at
			}
		}
		l := last.Sub(u.at)
		lat = append(lat, l)
		// the adaptive interval is never below its minimum: within 2 ×
		// MinRoundInterval meets AC-V1 whatever it was at the upload
		if l <= 2*reconciler.MinRoundInterval {
			r.v1Within++
		}
	}
	slices.Sort(lat)
	if len(lat) > 0 {
		r.latMedian = lat[len(lat)/2]
		r.latP95 = lat[min(len(lat)-1, int(math.Ceil(0.95*float64(len(lat))))-1)]
		r.latMax = lat[len(lat)-1]
	}

	for i := range h.nodes {
		d := classifyDeliveries(h.deliveries[i], h.placed[i], h.pushed[i], accPushRaceWindow)
		nr := &r.perNode[i]
		nr.dups, nr.dupBase, nr.dupRS, nr.pushRS = d.total, d.base, d.rs, d.pushRS
		r.dupTotal += d.total
		r.dupBase += d.base
		r.dupPushPS += d.pushPS
		r.dupPushRS += d.pushRS
		r.dupLatePushRS += d.latePushRS
		r.dupPS += d.ps
		r.dupRS += d.rs
		r.dupCross += d.cross
		r.raceMax = max(r.raceMax, d.raceMax)
	}
	fetchLat := slices.Clone(h.fetchLat)
	slices.Sort(fetchLat)
	if len(fetchLat) > 0 {
		r.fetchLatMedian = fetchLat[len(fetchLat)/2]
		r.fetchLatP99 = fetchLat[min(len(fetchLat)-1, int(math.Ceil(0.99*float64(len(fetchLat))))-1)]
		r.fetchLatMax = fetchLat[len(fetchLat)-1]
	}
	r.pushSpan = h.pushSpan
	return r
}

// accPushRaceWindow is how far from the push's Put to a node a protocol's
// first delivery of the pushed chunk to that node may land and still count
// as racing the push (classifyDeliveries). A delivery races the push in two
// ways. The push lands between the fetch's last reserve check (the claim's
// re-check) and the delivery: the delivery follows the push by less than the
// fetch's latency from that check. Or the fetch stores the chunk first, from
// a holder the same push reached a moment earlier: the delivery precedes the
// push by less than the push's span over its holders. The suite measures
// both in every run (fetch latency, push span); with latency 0 they stay in
// the tens of milliseconds (Decisions log), so 1 s leaves a wide margin and
// is still a fifth of MinRoundInterval. A delivery further from its push is
// one the node's reserve check should have stopped, a rangesync duplicate.
const accPushRaceWindow = time.Second

// accDups are a node's duplicate deliveries, by cause (classifyDeliveries).
type accDups struct {
	total          int // deliveries of an ID the node held or got already
	base           int // of base chunks: false differences
	pushPS, pushRS int // a protocol's first delivery of a pushed chunk that raced the push
	latePushRS     int // rangesync's first delivery of a pushed chunk past the window (in rs)
	ps, rs         int // a protocol's repeats; rs also counts latePushRS
	cross          int // fetched IDs that both protocols delivered
	raceMax        time.Duration
}

// classifyDeliveries classifies the deliveries of one node: a delivery of an
// ID the node holds or got already is a duplicate. Deliveries of a base chunk
// are false differences. The first delivery of a pushed chunk by a protocol
// raced the push when the node lacked the chunk at the protocol's last check
// (a session found it missing, or a pullsync offer came) and the push landed
// before the delivery; on a network that is pushsync racing a fetch (AC-D2).
// For rangesync, whose duplicates are asserted, it must land within window
// of the push's Put (accPushRaceWindow); past it, it counts as a rangesync
// repeat (latePushRS). Pullsync's are reported only and are not timed. The
// others are a protocol's repeated deliveries, or the second protocol's
// delivery of an ID the first delivered (cross-protocol). raceMax is the
// largest distance of a rangesync race from its push.
func classifyDeliveries(deliveries map[fingerprint.ID]*accDeliveries, placed map[fingerprint.ID]accPlacement, pushed map[fingerprint.ID]time.Time, window time.Duration) accDups {
	var c accDups
	for id, d := range deliveries {
		ps, rs := d.n[accViaPullsync], d.n[accViaRangesync]
		held := 0
		if placed[id] != 0 {
			held = 1
		}
		c.total += max(0, ps+rs+held-1)
		switch placed[id] {
		case accBase:
			c.base += ps + rs
			continue
		case accPush:
			c.pushPS += min(ps, 1)
			if rs > 0 {
				at, ok := pushed[id]
				if dist := d.first[accViaRangesync].Sub(at).Abs(); ok && dist <= window {
					c.pushRS++
					c.raceMax = max(c.raceMax, dist)
				} else {
					c.latePushRS++
					c.rs++
				}
			}
		default:
			if ps > 0 && rs > 0 {
				c.cross++
			}
		}
		c.ps += max(0, ps-1)
		c.rs += max(0, rs-1)
	}
	return c
}

// logAccTable logs the scenarios' results side by side, with the ratios to
// Baseline.
func logAccTable(t *testing.T, base, rs, mixed *accResult) {
	t.Helper()

	results := []*accResult{base, rs, mixed}
	first := slices.IndexFunc(results, func(r *accResult) bool { return r != nil })
	if first < 0 {
		return
	}
	var b strings.Builder
	row := func(name string, f func(r *accResult) string) {
		fmt.Fprintf(&b, "%-44s", name)
		for _, r := range results {
			v := "-"
			if r != nil {
				v = f(r)
			}
			fmt.Fprintf(&b, " | %-22s", v)
		}
		b.WriteString("\n")
	}
	ratio := func(name string, f func(r *accResult) float64) {
		fmt.Fprintf(&b, "%-44s", name+" / baseline")
		for _, r := range results {
			v := "-"
			if r != nil && base != nil && f(base) != 0 {
				v = fmt.Sprintf("%.2f", f(r)/f(base))
			}
			fmt.Fprintf(&b, " | %-22s", v)
		}
		b.WriteString("\n")
	}
	perUpload := func(r *accResult, v float64) float64 { return v / float64(max(1, r.uploads)) }
	metaAC := func(r *accResult) float64 { return perUpload(r, r.rsMetaIn+r.psOffered*102) }
	wireMeta := func(r *accResult) float64 {
		return perUpload(r, float64(r.wire["pullsync"]-min(r.wire["pullsync"], r.psDeliveryWire))+r.rsMetaOut)
	}
	chunkBytes := func(r *accResult) float64 { return perUpload(r, float64(r.psDeliveryWire)+r.rsChunksIn) }
	cpu := func(r *accResult) float64 { return 1000 * perUpload(r, r.cpu) }
	mb := func(v float64) string { return fmt.Sprintf("%.1f MB", v/1e6) }

	fmt.Fprintf(&b, "%-44s", "scenario")
	for _, r := range results {
		v := "-"
		if r != nil {
			v = r.name + " (" + r.modes + ")"
		}
		fmt.Fprintf(&b, " | %-22s", v)
	}
	b.WriteString("\n")
	row("uploads (rate/s)", func(r *accResult) string { return fmt.Sprintf("%d (%.0f)", r.uploads, r.rate) })
	row("window", func(r *accResult) string { return r.window.Round(time.Millisecond).String() })
	row("setup / total run", func(r *accResult) string {
		return r.setup.Round(time.Second).String() + " / " + r.run.Round(time.Second).String()
	})
	row("convergence after last upload", func(r *accResult) string { return r.converge.Round(time.Millisecond).String() })
	row("upload latency median / p95 / max", func(r *accResult) string {
		return fmt.Sprintf("%v/%v/%v", r.latMedian.Round(10*time.Millisecond), r.latP95.Round(10*time.Millisecond), r.latMax.Round(10*time.Millisecond))
	})
	row("uploads within 2xMinRoundInterval (AC-V1)", func(r *accResult) string { return fmt.Sprintf("%d/%d", r.v1Within, r.uploads) })
	row("round_interval_seconds min / max", func(r *accResult) string {
		if r.roundMax == 0 {
			return "-"
		}
		return fmt.Sprintf("%.1f / %.1f", r.roundMin, r.roundMax)
	})
	row("AC-C2 last session after last upload", func(r *accResult) string { return r.quiet.Round(time.Millisecond).String() })
	row("metadata B/upload (AC def: rs in + offers*102)", func(r *accResult) string { return fmt.Sprintf("%.0f", metaAC(r)) })
	ratio("metadata (AC def)", metaAC)
	row("metadata B/upload (wire)", func(r *accResult) string { return fmt.Sprintf("%.0f", wireMeta(r)) })
	ratio("metadata (wire)", wireMeta)
	row("chunk B/upload", func(r *accResult) string { return fmt.Sprintf("%.0f", chunkBytes(r)) })
	ratio("chunk bytes", chunkBytes)
	row("wire bytes pullsync / rangesync", func(r *accResult) string {
		return mb(float64(r.wire["pullsync"])) + " / " + mb(float64(r.wire["rangesync"]))
	})
	row("rangesync metric bytes out (meta+chunks)", func(r *accResult) string { return mb(r.rsMetaOut + r.rsChunksOut) })
	row("rangesync B/upload status/reconcile/fetch-meta", func(r *accResult) string {
		fetch := float64(r.wireStream["rangesync/fetch"]) - r.rsChunksOut
		return fmt.Sprintf("%.0f/%.0f/%.0f", perUpload(r, float64(r.wireStream["rangesync/status"])), perUpload(r, float64(r.wireStream["rangesync/reconcile"])), perUpload(r, fetch))
	})
	row("pullsync offered / wanted / delivered", func(r *accResult) string {
		return fmt.Sprintf("%.0f/%.0f/%.0f", r.psOffered, r.psWanted, r.psDelivered)
	})
	row("rangesync fetched", func(r *accResult) string { return fmt.Sprintf("%.0f", r.rsFetched) })
	row("duplicates total (pullsync/rangesync/cross)", func(r *accResult) string {
		return fmt.Sprintf("%d (%d/%d/%d)", r.dupTotal, r.dupPS, r.dupRS, r.dupCross)
	})
	row("deliveries racing a push pullsync/rangesync", func(r *accResult) string {
		return fmt.Sprintf("%d / %d (max %v)", r.dupPushPS, r.dupPushRS, r.raceMax.Round(time.Millisecond))
	})
	row("rangesync pushed-chunk deliveries past window", func(r *accResult) string { return strconv.Itoa(r.dupLatePushRS) })
	row("rs fetch latency median/p99/max; push span", func(r *accResult) string {
		if r.fetchLatMax == 0 {
			return "-"
		}
		ms := func(d time.Duration) string { return strconv.FormatInt(d.Milliseconds(), 10) }
		return fmt.Sprintf("%s/%s/%s; %s ms", ms(r.fetchLatMedian), ms(r.fetchLatP99), ms(r.fetchLatMax), ms(r.pushSpan))
	})
	row("deliveries of base chunks", func(r *accResult) string { return strconv.Itoa(r.dupBase) })
	row("duplicates (rangesync repeats; races) P / R / D", func(r *accResult) string {
		var dups, own, races [3]int
		for _, nr := range r.perNode {
			k := slices.Index([]accMode{accPullsyncOnly, accRangesyncOnly, accDual}, nr.mode)
			dups[k] += nr.dups
			own[k] += nr.dupRS
			races[k] += nr.pushRS
		}
		return fmt.Sprintf("%d / %d (%d; %d) / %d (%d; %d)", dups[0], dups[1], own[1], races[1], dups[2], own[2], races[2])
	})
	row("cross_protocol_duplicates_total", func(r *accResult) string { return fmt.Sprintf("%.0f", r.crossMetric) })
	row("CPU s per 1000 uploads", func(r *accResult) string { return fmt.Sprintf("%.2f", cpu(r)) })
	ratio("CPU", cpu)
	row("peak index memory node / sum (run node)", func(r *accResult) string {
		return mb(r.indexPeakNode) + " / " + mb(r.indexPeakSum) + " (" + mb(r.indexPeakRun) + ")"
	})
	row("peak live Go heap (process)", func(r *accResult) string { return mb(float64(r.heapPeak)) })
	row("pairwise_diff median (sessions)", func(r *accResult) string { return fmt.Sprintf("<=%v (%d)", r.diffMedian, r.diffCount) })
	row("pairwise_diff_half median / p99", func(r *accResult) string { return fmt.Sprintf("<=%v / <=%v", r.diffHalfMedian, r.diffHalfP99) })
	row("symbols_per_difference median/p95/mean (n)", func(r *accResult) string {
		return fmt.Sprintf("<=%v/<=%v/%.2f (%d)", r.symMedian, r.symP95, r.symMean, r.symCount)
	})
	row("decode_corruption / more_requests", func(r *accResult) string { return fmt.Sprintf("%.0f / %.0f", r.decodeCorruption, r.moreRequests) })
	row("IDs found missing per upload (p(N-p))", func(r *accResult) string {
		return fmt.Sprintf("%.1f (%d)", perUpload(r, r.wantsDecoded), 3*(r.nodes-3))
	})
	row("reconcile B per ID found missing", func(r *accResult) string {
		if r.wantsDecoded == 0 {
			return "-"
		}
		return fmt.Sprintf("%.0f", float64(r.wireStream["rangesync/reconcile"])/r.wantsDecoded)
	})
	row("parts_sent", func(r *accResult) string { return fmtCounts(r.parts) })
	row("sessions_total", func(r *accResult) string { return fmtCounts(r.sessions) })
	row("want_claims", func(r *accResult) string { return fmtCounts(r.wantClaims) })
	t.Logf("T6.1 acceptance, N=%d:\n%s", results[first].nodes, b.String())
}

func fmtCounts(m map[string]float64) string {
	keys := make([]string, 0, len(m))
	for k, v := range m {
		if v != 0 {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%.0f", k, m[k])
	}
	return strings.Join(parts, ",")
}
