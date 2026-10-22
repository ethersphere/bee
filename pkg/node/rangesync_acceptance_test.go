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
//     Index equals a brute force over its reserve (AC-C1), with zero
//     duplicate deliveries (AC-D1), no corrupt decode (AC-K3) and quiescence
//     within 120 s of the last upload (AC-C2);
//   - Mixed: every node converges (AC-C1), and every rangesync-only node
//     serves the pullsync peers that pull from it (AC-S3).
func TestAcceptance(t *testing.T) {
	for _, n := range accSizes(t) {
		t.Run(fmt.Sprintf("N=%d", n), func(t *testing.T) {
			seed := uint64(n)
			var baseline, rangesyncOnly, mixed *accResult
			t.Run("baseline", func(t *testing.T) {
				baseline = runAccScenario(t, "baseline", accOptions{Nodes: n, Seed: seed, Radius: 2})
				checkBaseline(t, baseline)
			})
			t.Run("rangesync", func(t *testing.T) {
				rangesyncOnly = runAccScenario(t, "rangesync", accOptions{Nodes: n, Seed: seed, Radius: 2, Mode: func(int) accMode { return accRangesyncOnly }})
				checkRangesyncOnly(t, rangesyncOnly)
			})
			t.Run("mixed", func(t *testing.T) {
				mixed = runAccScenario(t, "mixed", accOptions{Nodes: n, Seed: seed, Radius: 2, Mode: accMixedMode})
				checkMixed(t, mixed)
			})
			logAccTable(t, baseline, rangesyncOnly, mixed)
		})
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

	if r.dupTotal != 0 || r.crossMetric != 0 {
		t.Errorf("duplicate deliveries %d (cross_protocol_duplicates_total %v), want 0", r.dupTotal, r.crossMetric)
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
	checkNoFaults(t, r)
}

func checkMixed(t *testing.T, r *accResult) {
	t.Helper()

	for i, nr := range r.perNode {
		if nr.mode != accRangesyncOnly {
			continue
		}
		if nr.psSentOffered == 0 || nr.psSent == 0 {
			t.Errorf("rangesync-only node %d served pullsync peers %v offers and %v chunks, want both > 0", i, nr.psSentOffered, nr.psSent)
		}
		if nr.psDelivered != 0 {
			t.Errorf("rangesync-only node %d pulled %v chunks with pullsync", i, nr.psDelivered)
		}
	}
	if r.decodeCorruption != 0 {
		t.Errorf("decode_corruption_total %v, want 0", r.decodeCorruption)
	}
	checkNoFaults(t, r)
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
	dupTotal, dupPS, dupRS, dupCross             int
	crossMetric                                  float64
	wantClaims, sessions, parts                  map[string]float64
	wantsDecoded                                 float64 // Σ pairwise_diff_half: IDs sessions found missing
	symMedian, symP95, symMean                   float64
	symCount                                     uint64
	diffMedian, diffHalfMedian, diffHalfP99      float64
	diffCount                                    uint64
	decodeCorruption, moreRequests               float64
	disconnects, blocklists                      int64
	lastReason                                   string
	perNode                                      []accNodeResult
}

// accNodeResult is what one node measured.
type accNodeResult struct {
	mode                               accMode
	psSent, psSentOffered, psDelivered float64
	rsFetched, cross                   float64
	dups, dupRS                        int // duplicate deliveries, and rangesync's own
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

	// duplicates: a delivery of an ID the node holds already, placed by
	// the harness or delivered before. A protocol's own duplicates are its
	// deliveries of a placed ID and its repeated deliveries; the rest are
	// cross-protocol (the second protocol's delivery of an ID the first
	// delivered).
	for i := range h.nodes {
		for id, d := range h.deliveries[i] {
			held := 0
			if h.placed[i][id] {
				held = 1
			}
			own := func(n int) int {
				if held == 1 {
					return n
				}
				return max(0, n-1)
			}
			total := max(0, d[accViaPullsync]+d[accViaRangesync]+held-1)
			ps, rs := own(d[accViaPullsync]), own(d[accViaRangesync])
			r.dupTotal += total
			r.dupPS += ps
			r.dupRS += rs
			r.dupCross += total - ps - rs
			r.perNode[i].dups += total
			r.perNode[i].dupRS += rs
		}
	}
	return r
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
	row("duplicates (rangesync's own) at P / R / D", func(r *accResult) string {
		var dups, own [3]int
		for _, nr := range r.perNode {
			k := slices.Index([]accMode{accPullsyncOnly, accRangesyncOnly, accDual}, nr.mode)
			dups[k] += nr.dups
			own[k] += nr.dupRS
		}
		return fmt.Sprintf("%d / %d (%d) / %d (%d)", dups[0], dups[1], own[1], dups[2], own[2])
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
