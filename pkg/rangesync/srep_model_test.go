// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"container/heap"
	"encoding/binary"
	"fmt"
	"math"
	"math/bits"
	"math/rand/v2"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
)

// The analytical model of a neighborhood (proposal §13 "Neighborhood
// simulation", §7.8; T4.6 part 2), in the style of SREPSim: it follows the
// protocol's decisions and message sizes, not its bytes on a stream, so a
// run takes milliseconds and G6 can sweep its constants. It has three
// layers.
//
//   - The decoder, Monte Carlo over the real code (srepDecodeTable): for a
//     difference of d IDs, random IDs are encoded with the FloatMapper and
//     their coded symbols fed to fingerprint.Decoder one at a time; the
//     threshold K is the number of symbols after which it is Done. A batch
//     of n symbols decodes if and only if n ≥ K, since peeling more symbols
//     never recovers fewer IDs. The thresholds of each d are drawn once per
//     table and reused.
//   - A session, closed form given K (srepParams.stream, srepWire): the
//     responder's first batch FirstBatch(d̂) with d̂ = max(|cA − cB|, hint(R),
//     1), then a More up to GrowthFactor times the symbols received, within
//     cap(R) and SessionSymbolBudget, until the symbols reach K. The message
//     sizes are those of the wire encoders (bytes_metadata counts the
//     delimited encoding), with the expected length of each symbol's count
//     varint from its binomial count (srepWire.symbolData). Averaging over
//     the thresholds of d gives a session's expected cost (srepParams.ideal).
//   - The neighborhood, discrete events (srepSim, srepParams.run): N nodes,
//     each upload stored on p random nodes, and per node the Index's root
//     debounce (StatusMinInterval after the first change), Status gossip
//     (to each peer StatusMinInterval after the last send once the summary
//     changed, StatusMaxInterval otherwise), rounds (triggered by a root
//     change or a changed peer Status, deferred to MinRoundInterval after
//     the last round, and on the adaptive timer, MinRoundInterval to
//     MaxRoundInterval), the Statuses a round sends before its sessions, the
//     initiator rule and the takeover (2 round intervals plus a jitter of up
//     to one without a session of the initiator), MaxInitiatedSessions slots
//     with the recheck of a waiting session (stale when the digests now
//     match), one session at a time per pair, the session's legs with a
//     one-way Delay as the harness has them (the open's round trip, the
//     Fingerprint, the Symbols, a round trip per More, the Items, the empty
//     answer and the FullClose's round trip), the hint recorded by both
//     sides, the claims of the want set (each missing chunk fetched once)
//     and the fetch's legs (open, Want, Delivery). At Delay 0 a leg takes
//     srepInstant, so that concurrent calls progress in step as the
//     harness's goroutines do; events of one instant run in a random order.
//
// srepParams.ideal is the closed form of §7.8 with the session cost of the
// real decoder: synchronous rounds of MinRoundInterval in which every pair
// reconciles once on the round's snapshot. srepPullsyncACB1, srepPullsync78
// and srepPullsyncWire are the pullsync baselines (§2.3–2.4).
//
// Not modeled, against the harness (srep_model_sim_test.go measures what
// these gaps cost): a node's MaxConcurrentSessions cap and the peer session
// limit (Busy and session_limit answers; a handful per run at N = 17), a
// collision of a takeover with the initiator's own session (a takeover
// waits while this node serves the peer instead), the served-wants fetcher
// that serializes a peer's served fetches, splits and item exchanges (a
// stream that reaches cap(R) or a first batch past it is counted as
// Outside and resolved as if it decoded), Items pages past the first, the
// per-peer rate limits and the GCRA of the Status sender (an honest node
// never meets them), the fetch limiters and bandwidth (as in the harness),
// and the scheduler's interleaving within one instant beyond a random
// order of events.

// srepParams configures the model. Zero values select the defaults: the
// harness's schedule (simOptions) and the compiled protocol constants.
type srepParams struct {
	// Nodes is N and Replication is p (defaults 5 and 3).
	Nodes, Replication int
	// Seed seeds the upload holders, the order of the events of one
	// instant, the takeover jitters and the thresholds drawn per session.
	Seed uint64
	// Base is the number of elements every node holds at the start
	// (default 256); it only enters the counts.
	Base int
	// Batches batches of BatchSize uploads BatchInterval apart from Warmup
	// (defaults as simOptions), unless Rate is set: then Poisson uploads at
	// Rate per second for Duration (default 120 s) from Warmup.
	Batches, BatchSize    int
	BatchInterval, Warmup time.Duration
	Rate                  float64
	Duration              time.Duration
	// Tail is how long the run goes on after the last upload converged
	// (default 2 × MaxRoundInterval, the harness's), and Limit the most
	// time after the last upload before a run that has not converged fails
	// (default 10 minutes).
	Tail, Limit time.Duration
	// Delay is the one-way latency of every message, as simOptions.Delay.
	Delay time.Duration
	// MinBatch, BatchFactorTenths (BatchFactor × 10) and GrowthFactor are
	// the session constants (defaults: the compiled ones).
	MinBatch, BatchFactorTenths, GrowthFactor int
	// NoHints runs without hint(R) (simOptions.NoHints), NoClaims without
	// the want set's claims: every session's wants are fetched.
	NoHints, NoClaims bool
	// The gossip and round intervals (defaults: the compiled constants).
	StatusMinInterval, StatusMaxInterval time.Duration
	MinRoundInterval, MaxRoundInterval   time.Duration
	// InitiatedSessions is the number of sessions a node opens at once
	// (default reconciler.MaxInitiatedSessions).
	InitiatedSessions int
	// Root is the session root (default the harness's, radius 0), and
	// LeafBits the leaf bits of the Statuses (default the harness's, 8).
	Root     fingerprint.Range
	LeafBits uint8
}

// srepParamsOf returns the model's parameters for the harness's options o.
func srepParamsOf(o simOptions) srepParams {
	o = o.withDefaults()
	return srepParams{
		Nodes: o.Nodes, Replication: o.Replication, Seed: o.Seed, Base: o.Base,
		Batches: o.Batches, BatchSize: o.BatchSize, BatchInterval: o.BatchInterval, Warmup: o.Warmup,
		Tail: o.Tail, Limit: o.Limit, Delay: o.Delay, NoHints: o.NoHints,
		MinRoundInterval: o.MinRoundInterval, MaxRoundInterval: o.MaxRoundInterval,
	}.withDefaults()
}

func (p srepParams) withDefaults() srepParams {
	def := func(v *int, d int) {
		if *v == 0 {
			*v = d
		}
	}
	defDur := func(v *time.Duration, d time.Duration) {
		if *v == 0 {
			*v = d
		}
	}
	def(&p.Nodes, 5)
	def(&p.Replication, 3)
	def(&p.Base, 256)
	def(&p.Batches, 4)
	def(&p.BatchSize, 8)
	def(&p.MinBatch, rangesync.MinBatch)
	def(&p.BatchFactorTenths, int(math.Round(rangesync.BatchFactor*10)))
	def(&p.GrowthFactor, rangesync.GrowthFactor)
	def(&p.InitiatedSessions, reconciler.MaxInitiatedSessions)
	defDur(&p.BatchInterval, 17*time.Second)
	defDur(&p.Warmup, 61*time.Second)
	defDur(&p.Duration, 120*time.Second)
	defDur(&p.StatusMinInterval, rangesync.StatusMinInterval)
	defDur(&p.StatusMaxInterval, rangesync.StatusMaxInterval)
	defDur(&p.MinRoundInterval, rangesync.MinRoundInterval)
	defDur(&p.MaxRoundInterval, rangesync.MaxRoundInterval)
	p.MaxRoundInterval = max(p.MaxRoundInterval, p.MinRoundInterval)
	defDur(&p.Tail, 2*p.MaxRoundInterval)
	defDur(&p.Limit, 10*time.Minute)
	if p.LeafBits == 0 {
		p.LeafBits = serviceLeafBits
	}
	return p
}

// uploadRate returns the uploads per second.
func (p srepParams) uploadRate() float64 {
	if p.Rate > 0 {
		return p.Rate
	}
	return float64(p.BatchSize) / p.BatchInterval.Seconds()
}

// firstBatch returns the first batch for the estimate d:
// max(MinBatch, ⌈BatchFactor · d⌉ + 2), as rangesync.FirstBatch computes it.
func (p srepParams) firstBatch(d int) int {
	return max(p.MinBatch, (p.BatchFactorTenths*d+9)/10+2)
}

// capacity returns cap(R) of the root for a peer holding count elements in
// it, within the session's symbol budget: unlimited (the scan path) up to
// fingerprint.ScanLimit elements, the root cache above.
func srepCapacity(count int) int {
	if count <= fingerprint.ScanLimit {
		return rangesync.SessionSymbolBudget
	}
	return fingerprint.RootSymbols
}

// srepStream is what the decoding side of one symbol stream receives.
type srepStream struct {
	batches []int // the first batch, then each More's
	symbols int
	decoded bool // false: the stream reached its capacity first
}

// mores returns the number of More requests of the stream.
func (st srepStream) mores() int { return len(st.batches) - 1 }

// stream returns the stream of a session with first batch n0, decoder
// threshold k and capacity capR: Mores up to GrowthFactor times the symbols
// received (handleSymbols) until the symbols reach k or the stream cannot
// grow.
func (p srepParams) stream(n0, k, capR int) srepStream {
	n0 = min(n0, capR)
	st := srepStream{batches: []int{n0}, symbols: n0}
	for st.symbols < k {
		upTo := min(p.GrowthFactor*st.symbols, capR, rangesync.SessionSymbolBudget)
		if upTo <= st.symbols {
			return st
		}
		st.batches = append(st.batches, upTo-st.symbols)
		st.symbols = upTo
	}
	st.decoded = true
	return st
}

// srepDecodeTable holds, per difference size, sorted decoder thresholds
// drawn from the real decoder. It is safe for concurrent use.
type srepDecodeTable struct {
	mu    sync.Mutex
	m     fingerprint.Mapper
	draws int
	seed  uint64
	k     map[int][]int
}

func newSREPDecodeTable(draws int, seed uint64) *srepDecodeTable {
	return &srepDecodeTable{m: fingerprint.NewFloatMapper(), draws: draws, seed: seed, k: make(map[int][]int)}
}

// thresholds returns the sorted thresholds of a difference of d IDs; the
// slice must not be modified. No difference decodes with symbol 0.
func (t *srepDecodeTable) thresholds(tb testing.TB, d int) []int {
	tb.Helper()
	if d <= 0 {
		return []int{1}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if ks, ok := t.k[d]; ok {
		return ks
	}
	rng := newRand(t.seed<<32 ^ uint64(d))
	ks := make([]int, t.draws)
	for i := range ks {
		ks[i] = srepThreshold(tb, t.m, rng, d)
	}
	slices.Sort(ks)
	t.k[d] = ks
	return ks
}

// meanThreshold returns the mean threshold of a difference of d IDs.
func (t *srepDecodeTable) meanThreshold(tb testing.TB, d int) float64 {
	tb.Helper()
	sum := 0
	ks := t.thresholds(tb, d)
	for _, k := range ks {
		sum += k
	}
	return float64(sum) / float64(len(ks))
}

// srepThreshold returns the number of symbols after which the decoder
// recovers d random differences, split between the two sides.
func srepThreshold(tb testing.TB, m fingerprint.Mapper, rng *rand.Rand, d int) int {
	tb.Helper()
	ids := randIDs(rng, fingerprint.Range{}, d)
	for limit := 2*d + 64; ; limit *= 2 {
		remote := make([]fingerprint.CodedSymbol, limit)
		local := make([]fingerprint.CodedSymbol, limit)
		for i, id := range ids {
			if i%2 == 0 {
				fingerprint.Encode(m, remote, 0, id, 1)
			} else {
				fingerprint.Encode(m, local, 0, id, 1)
			}
		}
		dec := fingerprint.NewDecoder(m, fingerprint.Range{})
		for k := range limit {
			if err := dec.Add(uint64(k), remote[k:k+1], local[k:k+1]); err != nil {
				tb.Fatalf("decode %d differences: %v", d, err)
			}
			if dec.Done() {
				return k + 1
			}
		}
	}
}

// srepWire returns the delimited sizes of the messages a session, a fetch
// and the gossip send, as bytes_metadata counts them. It caches them, and is
// not safe for concurrent use.
type srepWire struct {
	root     fingerprint.Range
	leafBits uint8
	m        fingerprint.Mapper
	id       *pb.ID
	// cum holds, per count, the expected sizes of the symbols [0, i) of a
	// stream of count elements
	cum   map[int][]float64
	sizes map[[2]int]int // (kind, argument) to size
}

func newSREPWire(root fingerprint.Range, leafBits uint8) *srepWire {
	var id fingerprint.ID
	return &srepWire{
		root: root, leafBits: leafBits, m: fingerprint.NewFloatMapper(), id: rangesync.EncodeID(id),
		cum: make(map[int][]float64), sizes: make(map[[2]int]int),
	}
}

// cached returns the size of kind for arg, computing it with fn once.
func (w *srepWire) cached(kind, arg int, fn func() int) int {
	k := [2]int{kind, arg}
	if n, ok := w.sizes[k]; ok {
		return n
	}
	n := fn()
	w.sizes[k] = n
	return n
}

// srepSize returns the delimited size of m: its size and the varint of it.
func srepSize(m interface{ Size() int }) int {
	n := m.Size()
	return n + srepUvarintLen(n)
}

func srepUvarintLen(n int) int {
	return len(binary.AppendUvarint(nil, uint64(n)))
}

// srepField returns the size of a length-delimited field of n bytes with a
// one-byte tag.
func srepField(n int) int {
	return 1 + srepUvarintLen(n) + n
}

func (w *srepWire) message(p *pb.Part) int {
	return srepSize(&pb.Message{Parts: []*pb.Part{p}})
}

func (w *srepWire) summary(count int) fingerprint.Summary {
	s := fingerprint.Summary{Count: uint64(count)}
	s.Digest[0] = 1
	return s
}

// fp returns the size of an opening with the root's Fingerprint.
func (w *srepWire) fp(count int) int {
	return w.cached(srepFPBytes, count, func() int {
		return w.message(&pb.Part{Kind: &pb.Part_FP{FP: rangesync.EncodeFingerprint(w.root, w.summary(count))}})
	})
}

// symbolData returns the expected size of the data of symbols [start,
// start+n) of the stream of a random set of count elements: each symbol is
// its sum and checksum and the zigzag varint of its count less
// round(count·ρ(i)). Its count is binomial with p = ρ(i) = 2 / (i + 2), and
// the varint's expected length follows from the normal approximation.
func (w *srepWire) symbolData(count, start, n int) int {
	cum := w.cum[count]
	for i := len(cum); i <= start+n; i++ {
		if i == 0 {
			cum = append(cum, 0)
			continue
		}
		cum = append(cum, cum[i-1]+fingerprint.IDSize+fingerprint.ChecksumSize+srepVarintLen(w.m, count, i-1))
	}
	w.cum[count] = cum
	return int(math.Round(cum[start+n] - cum[start]))
}

// srepVarintLen returns the expected length of the count varint of symbol i
// of a random set of count elements.
func srepVarintLen(m fingerprint.Mapper, count, i int) float64 {
	rho := 2 / (float64(i) + 2)
	mean := float64(count)*rho - float64(m.Expected(uint64(count), uint64(i)))
	sd := math.Sqrt(float64(count) * rho * (1 - rho))
	if sd == 0 {
		v := int64(math.Round(mean))
		return float64(len(binary.AppendUvarint(nil, uint64(v<<1^v>>63))))
	}
	cdf := func(x float64) float64 { return 0.5 * math.Erfc(-(x-mean)/(sd*math.Sqrt2)) }
	within := func(lim float64) float64 { return cdf(lim-0.5) - cdf(-lim-0.5) } // v in [-lim, lim-1]
	p1 := within(1 << 6)
	p2 := within(1<<13) - p1
	p3 := within(1<<20) - p1 - p2
	return p1 + 2*p2 + 3*p3 + 4*(1-p1-p2-p3)
}

// symbols returns the size of a Message with one Symbols part of symbols
// [start, start+n) of a stream of count elements.
func (w *srepWire) symbols(count, start, n int) int {
	head := (&pb.Symbols{Range: rangesync.EncodeRange(w.root), Count: uint64(count), Start: uint64(start)}).Size()
	if data := w.symbolData(count, start, n); data > 0 {
		head += srepField(data)
	}
	msg := srepField(srepField(head))
	return msg + srepUvarintLen(msg)
}

// more returns the size of a Message with one More part.
func (w *srepWire) more(upTo int) int {
	return w.cached(srepMoreBytes, upTo, func() int {
		return w.message(&pb.Part{Kind: &pb.Part_More{More: rangesync.EncodeMore(rangesync.More{Range: w.root, UpTo: uint64(upTo)})}})
	})
}

// items returns the size of a Message with one Items page of k IDs.
func (w *srepWire) items(k int) int {
	return w.cached(srepItemBytes, k, func() int {
		it := &pb.Items{Range: rangesync.EncodeRange(w.root), IDs: make([]*pb.ID, k)}
		for i := range it.IDs {
			it.IDs[i] = w.id
		}
		return w.message(&pb.Part{Kind: &pb.Part_Items{Items: it}})
	})
}

// empty returns the size of an empty Message.
func (w *srepWire) empty() int {
	return srepSize(&pb.Message{})
}

// want returns the size of a Want of k IDs.
func (w *srepWire) want(k int) int {
	return w.cached(srepWantBytes, k, func() int {
		want := &pb.Want{IDs: make([]*pb.ID, k)}
		for i := range want.IDs {
			want.IDs[i] = w.id
		}
		return srepSize(want)
	})
}

// status returns the size of a Status of a node holding count elements.
func (w *srepWire) status(count int) int {
	return w.cached(srepStatusBytes, count, func() int {
		return srepSize(rangesync.EncodeStatus(rangesync.Status{Root: w.root, Summary: w.summary(count), LeafBits: w.leafBits, Mapping: fingerprint.DefaultMapperID}))
	})
}

// The kinds of metadata bytes the model counts.
const (
	srepStatusBytes = iota
	srepFPBytes
	srepSymbolBytes
	srepMoreBytes
	srepItemBytes
	srepEmptyBytes
	srepWantBytes
	srepKinds
)

var srepKindNames = [srepKinds]string{"status", "fingerprint", "symbols", "more", "items", "empty", "want"}

// srepResult is what one run of the model measured, from the first upload
// to the end of the run, as the harness counts.
type srepResult struct {
	Nodes, Replication int
	// Uploads counts the uploads.
	Uploads int
	// Bytes holds the metadata bytes by kind (srepKindNames), each message
	// counted once.
	Bytes [srepKinds]float64
	// Sessions counts the completed sessions, Equal those that found equal
	// digests, Decoded those that decoded a stream (ZeroDecoded of them
	// with no difference), Stale those a recheck ended before they opened,
	// and Takeovers the sessions the higher overlay opened.
	Sessions, Equal, Decoded, ZeroDecoded, Stale, Takeovers int
	// Symbols, Recovered and Mores sum over the decoded streams.
	Symbols, Recovered, Mores int
	// Diffs holds n_Δ of every completed session (0 for an equal one),
	// MoreCounts the Mores of every decoded stream and SymbolsPerDiff the
	// symbols per difference of every decoded stream with a difference.
	Diffs          []int
	MoreCounts     []int
	SymbolsPerDiff []float64
	// Fetches counts the fetches (Want streams), Fetched the chunks they
	// stored, Denied the claims the want set denied and Duplicates the
	// deliveries of a chunk the node held already.
	Fetches, Fetched, Denied, Duplicates int
	// Convergence holds each upload's time to reach every node, and
	// Batches each batch's time and the most rounds a node ran meanwhile
	// (simBatch's rules).
	Convergence []time.Duration
	Batches     []simBatch
	// Outside counts the streams outside the model's domain: a first batch
	// past cap(R) or a stream that reached it before it decoded.
	Outside int
}

// Metadata returns the metadata bytes of the run.
func (r *srepResult) Metadata() float64 {
	total := 0.0
	for _, b := range r.Bytes {
		total += b
	}
	return total
}

// MetadataPerUpload returns the metadata bytes per upload.
func (r *srepResult) MetadataPerUpload() float64 {
	return r.Metadata() / float64(r.Uploads)
}

// Share returns the metadata per upload as a share of AC-B1's pullsync
// baseline.
func (r *srepResult) Share() float64 {
	return r.MetadataPerUpload() / srepPullsyncACB1(r.Nodes)
}

// SymbolsPerDifference returns the symbols the decoding sides received per
// difference they recovered, as simResult.symbolsPerDifference.
func (r *srepResult) SymbolsPerDifference() float64 {
	return float64(r.Symbols) / float64(max(r.Recovered, 1))
}

// DiffPerUpload returns n_Δ per upload: the differences of all sessions
// (each session once) per upload.
func (r *srepResult) DiffPerUpload() float64 {
	sum := 0
	for _, d := range r.Diffs {
		sum += d
	}
	return float64(sum) / float64(r.Uploads)
}

// MoresPerUpload returns the More requests per upload.
func (r *srepResult) MoresPerUpload() float64 {
	return float64(r.Mores) / float64(r.Uploads)
}

// RoundTrips returns the mean round trips of a decoded session, counting
// the open and the close: 4 without a More, one more per More.
func (r *srepResult) RoundTrips() float64 {
	return 4 + float64(r.Mores)/float64(max(r.Decoded, 1))
}

// MoreShares returns the shares of the decoded streams with no More, one,
// and more than one.
func (r *srepResult) MoreShares() (none, one, more float64) {
	for _, k := range r.MoreCounts {
		switch k {
		case 0:
			none++
		case 1:
			one++
		default:
			more++
		}
	}
	n := float64(max(len(r.MoreCounts), 1))
	return none / n, one / n, more / n
}

// MaxConvergence returns the longest time an upload took to converge.
func (r *srepResult) MaxConvergence() time.Duration {
	return slices.Max(r.Convergence)
}

// ConvergenceQuantile returns the q-quantile of the uploads' convergence.
func (r *srepResult) ConvergenceQuantile(q float64) time.Duration {
	s := slices.Sorted(slices.Values(r.Convergence))
	return s[max(0, int(math.Ceil(q*float64(len(s))))-1)]
}

// DiffQuantile returns the q-quantile of n_Δ per session.
func (r *srepResult) DiffQuantile(q float64) float64 {
	xs := make([]float64, len(r.Diffs))
	for i, d := range r.Diffs {
		xs[i] = float64(d)
	}
	return quantile(xs, q)
}

// SymbolsPerDiffQuantile returns the q-quantile of the symbols per
// difference of the decoded streams with a difference.
func (r *srepResult) SymbolsPerDiffQuantile(q float64) float64 {
	return quantile(r.SymbolsPerDiff, q)
}

func (r *srepResult) String() string {
	none, one, _ := r.MoreShares()
	parts := ""
	for k, b := range r.Bytes {
		parts += fmt.Sprintf(" %s %.0f", srepKindNames[k], b/float64(r.Uploads))
	}
	return fmt.Sprintf("metadata %.0f B per upload (%.0f%% of AC-B1's pullsync; per upload:%s); n_Δ %.1f per upload, p50 %.0f per session; "+
		"%.2f symbols per difference (per stream p10 %.2f, p50 %.2f, p90 %.2f), %.2f Mores per upload, %.2f round trips per decoded session (%.0f%% no More, %.0f%% one); "+
		"sessions %d (%d equal, %d decoded, %d of them zero, %d stale, %d takeovers); %d fetched, %d denied, %d duplicates; "+
		"convergence p50 %v, max %v; %d outside",
		r.MetadataPerUpload(), 100*r.Share(), parts, r.DiffPerUpload(), r.DiffQuantile(0.5),
		r.SymbolsPerDifference(), r.SymbolsPerDiffQuantile(0.1), r.SymbolsPerDiffQuantile(0.5), r.SymbolsPerDiffQuantile(0.9),
		r.MoresPerUpload(), r.RoundTrips(), 100*none, 100*one,
		r.Sessions, r.Equal, r.Decoded, r.ZeroDecoded, r.Stale, r.Takeovers, r.Fetched, r.Denied, r.Duplicates,
		r.ConvergenceQuantile(0.5), r.MaxConvergence(), r.Outside)
}

// run runs the neighborhood model once.
func (p srepParams) run(tb testing.TB, table *srepDecodeTable) *srepResult {
	tb.Helper()
	p = p.withDefaults()
	s := newSREPSim(tb, p, table)
	s.run()
	return &s.res
}

// srepNever is a time that never comes.
const srepNever = time.Duration(math.MaxInt64)

// srepInstant is the time of a message leg at Delay 0. In the harness the
// calls of one instant run concurrently in real time, each leg a goroutine
// hand-off, so they progress in step: every session of a round reads the
// indexes before any of them ends and fetches. A random order of the legs
// of one instant would instead let a fetch land before a concurrent
// session's read, which the harness does not show (n_Δ per upload p(N − p)
// at Delay 0). A leg of a microsecond keeps the step order and adds
// microseconds to the times.
const srepInstant = time.Microsecond

// srepEvent is a pending event: fn runs at at. Events of one instant run in
// the order of tie, a random number, then of seq.
type srepEvent struct {
	at       time.Duration
	tie, seq uint64
	fn       func()
}

type srepQueue []*srepEvent

func (q srepQueue) Len() int { return len(q) }
func (q srepQueue) Less(i, j int) bool {
	a, b := q[i], q[j]
	if a.at != b.at {
		return a.at < b.at
	}
	if a.tie != b.tie {
		return a.tie < b.tie
	}
	return a.seq < b.seq
}
func (q srepQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *srepQueue) Push(x any)   { *q = append(*q, x.(*srepEvent)) }
func (q *srepQueue) Pop() any {
	old := *q
	e := old[len(old)-1]
	old[len(old)-1] = nil
	*q = old[:len(old)-1]
	return e
}

// srepStatus is what a Status says of its sender's root: its count and
// digest (the digest is a hash of the held uploads).
type srepStatus struct {
	count  int
	digest uint64
}

// srepPeer is a node's state for one peer.
type srepPeer struct {
	// gossip: picked is set once a Status was picked for the peer,
	// lastSent and lastSentSt are the last pick, sending is set while it
	// is in flight and sentAny once one reached the peer
	picked, sending, sentAny bool
	lastSent                 time.Duration
	lastSentSt               srepStatus
	// the peer's last Status
	have   bool
	status srepStatus
	// busy is set while a session this node opened with the peer, or its
	// fetch, runs (the peer's machine is not Idle); serving while this node
	// serves a session of the peer
	busy, serving bool
	// the takeover wait of a peer that initiates: since takeoverAt, with
	// jitter; servedAt is the last session the peer ran with this node
	takeover             bool
	takeoverAt, servedAt time.Duration
	jitter               float64
	// hint is hint(R) of the root with the peer
	hint int
}

// srepNode is one node of the model.
type srepNode struct {
	i          int
	held       []uint64 // the uploads it holds, a bit set
	count      int      // Base + held
	digest     uint64
	debouncing bool
	peers      []srepPeer
	// the round loop: the last round's start, the interval, the round
	// timer and a deferred trigger (srepNever when none), the generations
	// that cancel stale events
	last, interval      time.Duration
	timerAt, deferredAt time.Duration
	roundGen, gossipGen uint64
	foundDiff           bool
	// the initiated-session slots and the sessions waiting for one
	slots   int
	waiting []func()
	// claims holds the uploads claimed in the want set
	claims map[int]bool
	rounds []time.Duration
}

func (n *srepNode) has(u int) bool { return n.held[u/64]>>(u%64)&1 == 1 }

func (n *srepNode) status() srepStatus { return srepStatus{count: n.count, digest: n.digest} }

// srepUpload is one upload: when, its batch, how many nodes hold it and
// when the last of them stored it.
type srepUpload struct {
	at        time.Duration
	batch     int
	holders   int
	converged time.Duration
}

// srepSession is one session between initiator a and responder b.
type srepSession struct {
	a, b         *srepNode
	takeover     bool
	cA, cB       int        // the counts of a's Fingerprint and b's stream
	dA           uint64     // the digest of a's Fingerprint
	sb           []uint64   // b's held uploads when it decided (its stream's snapshot)
	n0, capR     int        // the first batch and cap(R)
	st           srepStream // the stream a decoded
	onlyA, onlyB []int
}

// srepSim is one run of the neighborhood model.
type srepSim struct {
	tb       testing.TB
	p        srepParams
	table    *srepDecodeTable
	wire     *srepWire
	rng      *rand.Rand
	leg      time.Duration // one message leg: Delay, or srepInstant at Delay 0
	now      time.Duration
	queue    srepQueue
	seq      uint64
	nodes    []*srepNode
	keys     []uint64 // digest keys of the uploads
	ups      []srepUpload
	batchAt  []time.Duration
	counting bool // from the first upload on
	issued   int  // uploads stored so far
	pending  int  // uploads issued and not converged
	lastConv time.Duration
	res      srepResult
}

func newSREPSim(tb testing.TB, p srepParams, table *srepDecodeTable) *srepSim {
	tb.Helper()
	s := &srepSim{tb: tb, p: p, table: table, wire: newSREPWire(p.Root, p.LeafBits), rng: newRand(p.Seed ^ 0x5eed), leg: max(p.Delay, srepInstant)}
	s.res.Nodes, s.res.Replication = p.Nodes, p.Replication

	// the upload schedule
	if p.Rate > 0 {
		for t := p.Warmup; t < p.Warmup+p.Duration; t += time.Duration(s.rng.ExpFloat64() / p.Rate * float64(time.Second)) {
			s.ups = append(s.ups, srepUpload{at: t, batch: len(s.ups)})
		}
	} else {
		for b := range p.Batches {
			for range p.BatchSize {
				s.ups = append(s.ups, srepUpload{at: p.Warmup + time.Duration(b)*p.BatchInterval, batch: b})
			}
		}
	}
	s.keys = make([]uint64, len(s.ups))
	for i := range s.keys {
		s.keys[i] = s.rng.Uint64()
	}
	words := (len(s.ups) + 63) / 64
	for i := range p.Nodes {
		n := &srepNode{
			i: i, held: make([]uint64, words), count: p.Base, peers: make([]srepPeer, p.Nodes),
			interval: p.MinRoundInterval, timerAt: srepNever, deferredAt: srepNever,
			slots: p.InitiatedSessions, claims: make(map[int]bool),
		}
		s.nodes = append(s.nodes, n)
	}
	return s
}

// at schedules fn at t.
func (s *srepSim) at(t time.Duration, fn func()) {
	s.seq++
	heap.Push(&s.queue, &srepEvent{at: t, tie: s.rng.Uint64(), seq: s.seq, fn: fn})
}

// count adds b bytes of kind k, from the first upload on.
func (s *srepSim) count(k, b int) {
	if s.counting {
		s.res.Bytes[k] += float64(b)
	}
}

func (s *srepSim) run() {
	p := s.p
	// the uploads, one event per instant
	for i := 0; i < len(s.ups); {
		j := i
		for j < len(s.ups) && s.ups[j].at == s.ups[i].at {
			j++
		}
		first, last := i, j
		s.at(s.ups[i].at, func() { s.upload(first, last) })
		i = j
	}
	// each node starts with a gossip pass (first contacts) and a round
	for _, n := range s.nodes {
		s.at(0, func() {
			s.gossip(n)
			s.round(n)
		})
	}
	lastUpload := s.ups[len(s.ups)-1].at
	end := srepNever
	for s.queue.Len() > 0 {
		e := heap.Pop(&s.queue).(*srepEvent)
		if e.at > end {
			break
		}
		s.now = e.at
		e.fn()
		if end == srepNever && s.issued == len(s.ups) && s.pending == 0 {
			end = s.lastConv + p.Tail
		}
		if end == srepNever && s.now > lastUpload+p.Limit {
			s.tb.Fatalf("model: %d uploads not converged %v after the last upload", s.pending, p.Limit)
		}
	}

	s.res.Uploads = len(s.ups)
	for _, u := range s.ups {
		s.res.Convergence = append(s.res.Convergence, u.converged-u.at)
	}
	for b, at := range s.batchAt {
		conv := at
		for _, u := range s.ups {
			if u.batch == b && u.converged > conv {
				conv = u.converged
			}
		}
		rounds := 0
		for _, n := range s.nodes {
			k := 0
			for _, r := range n.rounds {
				if r >= at && r <= conv {
					k++
				}
			}
			rounds = max(rounds, k)
		}
		s.res.Batches = append(s.res.Batches, simBatch{At: at, Convergence: conv - at, Rounds: rounds})
	}
}

// upload pushes uploads [first, last) to p random nodes each.
func (s *srepSim) upload(first, last int) {
	s.counting = true
	s.batchAt = append(s.batchAt, s.now)
	for u := first; u < last; u++ {
		s.issued++
		s.pending++
		for _, i := range s.rng.Perm(len(s.nodes))[:s.p.Replication] {
			s.store(s.nodes[i], u, false)
		}
	}
}

// store puts upload u into n's reserve, by a push or (fetched) a fetch.
func (s *srepSim) store(n *srepNode, u int, fetched bool) {
	if n.has(u) {
		if fetched {
			s.res.Duplicates++
		}
		return
	}
	n.held[u/64] |= 1 << (u % 64)
	n.count++
	n.digest ^= s.keys[u]
	if fetched {
		s.res.Fetched++
	}
	up := &s.ups[u]
	if up.holders++; up.holders == len(s.nodes) {
		up.converged = s.now
		s.pending--
		s.lastConv = s.now
	}
	// the Index's debounce: the first change signals StatusMinInterval later
	if !n.debouncing {
		n.debouncing = true
		s.at(s.now+s.p.StatusMinInterval, func() { s.rootChanged(n) })
	}
}

// rootChanged is the Index's root-change signal: a gossip pass and a round
// trigger (gossipLoop).
func (s *srepSim) rootChanged(n *srepNode) {
	n.debouncing = false
	s.gossip(n)
	s.trigger(n)
}

// statusDue returns when n's Status local is next due to peer j
// (reconciler statusDue), false while one is in flight.
func (s *srepSim) statusDue(n *srepNode, j int, local srepStatus) (time.Duration, bool) {
	ps := &n.peers[j]
	switch {
	case ps.sending:
		return 0, false
	case !ps.picked:
		return 0, true
	case ps.lastSentSt != local:
		return ps.lastSent + s.p.StatusMinInterval, true
	default:
		return ps.lastSent + s.p.StatusMaxInterval, true
	}
}

// sendDue sends n's Status local to every peer it is due to and returns how
// many it sent. A Status reaches the peer after the open's round trip and
// its leg (3 × Delay), and the send ends with the FullClose's round trip.
func (s *srepSim) sendDue(n *srepNode, local srepStatus) int {
	sent := 0
	for j := range s.nodes {
		if j == n.i {
			continue
		}
		if at, ok := s.statusDue(n, j, local); !ok || at > s.now {
			continue
		}
		ps := &n.peers[j]
		ps.picked, ps.sending, ps.lastSent, ps.lastSentSt = true, true, s.now, local
		s.count(srepStatusBytes, s.wire.status(local.count))
		to := s.nodes[j]
		s.at(s.now+3*s.leg, func() { s.statusArrived(to, n.i, local) })
		s.at(s.now+4*s.leg, func() { ps.sending, ps.sentAny = false, true })
		sent++
	}
	return sent
}

// gossip is a pass of n's gossip loop: it sends the Statuses that are due
// and arms the loop's timer for the next one.
func (s *srepSim) gossip(n *srepNode) {
	local := n.status()
	s.sendDue(n, local)
	next := srepNever
	for j := range s.nodes {
		if j == n.i {
			continue
		}
		if at, ok := s.statusDue(n, j, local); ok && at < next {
			next = at
		}
	}
	n.gossipGen++
	if next != srepNever {
		gen := n.gossipGen
		s.at(max(next, s.now), func() {
			if gen == n.gossipGen {
				s.gossip(n)
			}
		})
	}
}

// statusArrived records peer from's Status st at n; a changed one wakes n's
// gossip and triggers a round.
func (s *srepSim) statusArrived(n *srepNode, from int, st srepStatus) {
	ps := &n.peers[from]
	changed := !ps.have || ps.status != st
	ps.have, ps.status = true, st
	if changed {
		s.gossip(n)
		s.trigger(n)
	}
}

// trigger defers a round of n to MinRoundInterval after its last one.
func (s *srepSim) trigger(n *srepNode) {
	if n.deferredAt == srepNever {
		n.deferredAt = max(s.now, n.last+s.p.MinRoundInterval)
		s.armRound(n)
	}
}

// armRound schedules n's next round at its timer or deferred trigger.
func (s *srepSim) armRound(n *srepNode) {
	n.roundGen++
	next := min(n.timerAt, n.deferredAt)
	if next == srepNever {
		return
	}
	gen := n.roundGen
	s.at(next, func() {
		if gen == n.roundGen {
			s.round(n)
		}
	})
}

// round is one round of n: it sends the Statuses that are due and waits
// for them (track), then starts the sessions (schedule) and re-arms the
// timer at the adapted interval.
func (s *srepSim) round(n *srepNode) {
	n.last, n.timerAt, n.deferredAt = s.now, srepNever, srepNever
	n.roundGen++
	n.rounds = append(n.rounds, s.now)
	local := n.status()
	cont := func() {
		s.gossip(n)
		s.schedule(n, local)
		n.timerAt = s.now + n.interval
		s.armRound(n)
	}
	if s.sendDue(n, local) > 0 {
		s.at(s.now+4*s.leg, cont)
		return
	}
	cont()
}

// schedule starts n's sessions with every peer whose last Status differs
// from local and that n initiates with (the lower overlay), or takes over,
// and adapts the round interval.
func (s *srepSim) schedule(n *srepNode, local srepStatus) {
	differs := false
	var cands []int
	for j := range s.nodes {
		if j == n.i {
			continue
		}
		ps := &n.peers[j]
		if !ps.sentAny || !ps.have || ps.status == local {
			ps.takeover = false
			continue
		}
		differs = true
		if j < n.i { // the peer initiates
			if !ps.takeover {
				ps.takeover, ps.takeoverAt, ps.jitter = true, s.now, s.rng.Float64()
				continue
			}
			if wait := 2*n.interval + time.Duration(ps.jitter*float64(n.interval)); s.now < ps.takeoverAt+wait || ps.serving {
				continue
			}
		}
		if ps.busy {
			continue
		}
		cands = append(cands, j)
	}
	s.rng.Shuffle(len(cands), func(a, b int) { cands[a], cands[b] = cands[b], cands[a] })
	decided := s.now
	for _, j := range cands {
		n.peers[j].busy = true
		b := s.nodes[j]
		s.acquire(n, func() { s.startSession(n, b, decided) })
	}
	if differs || n.foundDiff {
		n.interval = max(s.p.MinRoundInterval, n.interval/2)
	} else {
		n.interval = min(s.p.MaxRoundInterval, 2*n.interval)
	}
	n.foundDiff = false
}

// acquire runs start once n has an initiated-session slot for it.
func (s *srepSim) acquire(n *srepNode, start func()) {
	if n.slots > 0 {
		n.slots--
		start()
		return
	}
	n.waiting = append(n.waiting, start)
}

// release frees one of n's slots, to the first waiting session if any.
func (s *srepSim) release(n *srepNode) {
	if len(n.waiting) > 0 {
		start := n.waiting[0]
		n.waiting = n.waiting[1:]
		start()
		return
	}
	n.slots++
}

// markServed records that a session with the peer of ps completed: a
// takeover waits again from now.
func (s *srepSim) markServed(ps *srepPeer) {
	ps.servedAt = s.now
	if ps.takeover {
		ps.takeoverAt, ps.jitter = s.now, s.rng.Float64()
	}
}

// startSession opens a session of a with b once it holds a slot, unless the
// round's decision no longer holds (recheckSession): the digests match now,
// or b ran a session with a since a decided to take over.
func (s *srepSim) startSession(a, b *srepNode, decided time.Duration) {
	ps := &a.peers[b.i]
	takeover := b.i < a.i
	if ps.status == a.status() || takeover && ps.servedAt > decided {
		if s.counting {
			s.res.Stale++
		}
		ps.busy = false
		s.release(a)
		return
	}
	sess := &srepSession{a: a, b: b, takeover: takeover}
	if takeover && s.counting {
		s.res.Takeovers++
	}
	d := s.leg
	// the open's round trip, then the Fingerprint's leg
	s.at(s.now+2*d, func() {
		sess.cA, sess.dA = a.count, a.digest
		s.count(srepFPBytes, s.wire.fp(sess.cA))
		s.at(s.now+d, func() { s.decide(sess) })
	})
}

// decide is the responder's answer to the opening.
func (s *srepSim) decide(sess *srepSession) {
	a, b, d := sess.a, sess.b, s.leg
	ps := &b.peers[a.i]
	ps.serving = true
	if b.count == sess.cA && b.digest == sess.dA {
		// equal digests: an empty answer, then the close
		s.count(srepEmptyBytes, s.wire.empty())
		s.at(s.now+2*d, func() { s.servedDone(b, a, nil) })
		s.at(s.now+3*d, func() { s.initiatorDone(sess, nil, true) })
		return
	}
	sess.cB = b.count
	sess.sb = slices.Clone(b.held)
	est := max(sess.cA, sess.cB) - min(sess.cA, sess.cB)
	if !s.p.NoHints {
		est = max(est, ps.hint)
	}
	sess.n0 = s.p.firstBatch(max(est, 1))
	sess.capR = min(srepCapacity(sess.cA), srepCapacity(sess.cB))
	if sess.n0 > sess.capR || sess.n0 > rangesync.MaxBatchSymbols {
		s.res.Outside++ // the root would be split
	}
	s.at(s.now+d, func() { s.decode(sess) })
}

// decode is the initiator decoding the responder's stream against its own
// snapshot, taken when the first batch arrives.
func (s *srepSim) decode(sess *srepSession) {
	a := sess.a
	for w := range a.held {
		onlyA, onlyB := a.held[w]&^sess.sb[w], sess.sb[w]&^a.held[w]
		for ; onlyA != 0; onlyA &= onlyA - 1 {
			sess.onlyA = append(sess.onlyA, w*64+bits.TrailingZeros64(onlyA))
		}
		for ; onlyB != 0; onlyB &= onlyB - 1 {
			sess.onlyB = append(sess.onlyB, w*64+bits.TrailingZeros64(onlyB))
		}
	}
	diff := len(sess.onlyA) + len(sess.onlyB)
	ks := s.table.thresholds(s.tb, diff)
	sess.st = s.p.stream(sess.n0, ks[s.rng.IntN(len(ks))], sess.capR)
	start := 0
	for i, n := range sess.st.batches {
		if i > 0 {
			s.count(srepMoreBytes, s.wire.more(start+n))
		}
		s.count(srepSymbolBytes, s.wire.symbols(sess.cB, start, n))
		start += n
	}
	if !sess.st.decoded {
		s.res.Outside++ // the stream would end in a split
	}
	if s.counting {
		s.res.Sessions++
		s.res.Decoded++
		s.res.Symbols += sess.st.symbols
		s.res.Recovered += diff
		s.res.Mores += sess.st.mores()
		s.res.Diffs = append(s.res.Diffs, diff)
		s.res.MoreCounts = append(s.res.MoreCounts, sess.st.mores())
		if diff == 0 {
			s.res.ZeroDecoded++
		} else {
			s.res.SymbolsPerDiff = append(s.res.SymbolsPerDiff, float64(sess.st.symbols)/float64(diff))
		}
	}
	s.at(s.now+2*time.Duration(sess.st.mores())*s.leg, func() { s.decoded(sess) })
}

// decoded is the initiator once its decoder is Done: it records the hint,
// keeps the IDs only the responder holds that it still lacks as wants, and
// lists the IDs only it holds.
func (s *srepSim) decoded(sess *srepSession) {
	a, b, d := sess.a, sess.b, s.leg
	if !s.p.NoHints {
		a.peers[b.i].hint = len(sess.onlyA) + len(sess.onlyB)
	}
	var wantsA []int
	for _, u := range sess.onlyB {
		if !a.has(u) {
			wantsA = append(wantsA, u)
		}
	}
	s.count(srepItemBytes, s.wire.items(len(sess.onlyA)))
	s.at(s.now+d, func() {
		// the responder handles the listing: the served hint, its wants
		// and the empty answer; then the close
		listed := len(sess.onlyA)
		common := sess.cA - min(listed, sess.cA)
		served := listed
		if sess.cB > common {
			served += sess.cB - common
		}
		if !s.p.NoHints {
			b.peers[a.i].hint = min(served, sess.st.symbols)
		}
		var wantsB []int
		for _, u := range sess.onlyA {
			if !b.has(u) {
				wantsB = append(wantsB, u)
			}
		}
		s.count(srepEmptyBytes, s.wire.empty())
		s.at(s.now+2*d, func() { s.servedDone(b, a, wantsB) })
		s.at(s.now+3*d, func() { s.initiatorDone(sess, wantsA, false) })
	})
}

// servedDone is the end of b's handler for a's session: the served
// session marks a, and b fetches its wants from a.
func (s *srepSim) servedDone(b, a *srepNode, wants []int) {
	ps := &b.peers[a.i]
	ps.serving = false
	s.markServed(ps)
	if len(wants) > 0 {
		b.foundDiff = true
		s.fetch(b, wants, nil)
	}
}

// initiatorDone is the end of a's session (its FullClose returned): the
// slot is freed and a fetches its wants from b; the peer is Idle again once
// the fetch ended.
func (s *srepSim) initiatorDone(sess *srepSession, wants []int, equal bool) {
	a, b := sess.a, sess.b
	s.release(a)
	ps := &a.peers[b.i]
	s.markServed(ps)
	if equal && s.counting {
		s.res.Sessions++
		s.res.Equal++
		s.res.Diffs = append(s.res.Diffs, 0)
	}
	if len(wants) > 0 {
		a.foundDiff = true
	}
	s.fetch(a, wants, func() { ps.busy = false })
}

// fetch claims the wants n does not hold in its want set (unless NoClaims)
// and fetches the granted ones from the session's peer: the open's round
// trip, the Want and the Deliveries, 4 × Delay; done runs at the end.
func (s *srepSim) fetch(n *srepNode, wants []int, done func()) {
	var granted []int
	for _, u := range wants {
		if n.has(u) {
			continue
		}
		if !s.p.NoClaims {
			if n.claims[u] {
				s.res.Denied++
				continue
			}
			n.claims[u] = true
		}
		granted = append(granted, u)
	}
	if len(granted) == 0 {
		if done != nil {
			done()
		}
		return
	}
	s.res.Fetches++
	for i := 0; i < len(granted); i += rangesync.MaxWantIDs {
		s.count(srepWantBytes, s.wire.want(min(rangesync.MaxWantIDs, len(granted)-i)))
	}
	s.at(s.now+4*s.leg, func() {
		for _, u := range granted {
			if !s.p.NoClaims {
				delete(n.claims, u)
			}
			s.store(n, u, true)
		}
		if done != nil {
			done()
		}
	})
}

// srepCost is the expected cost of one symbol stream of a difference of d
// IDs with the estimate e, over the decoder's thresholds of d.
type srepCost struct {
	// Symbols and Mores are the expected symbols received and More
	// requests, and SymbolBytes and MoreBytes their Messages' bytes.
	Symbols, Mores, SymbolBytes, MoreBytes float64
	// MoreShares holds the probabilities of 0, 1, 2 and more Mores, and
	// Undecoded that the stream reaches cap(R) first.
	MoreShares [4]float64
	Undecoded  float64
}

// RoundTrips returns the expected round trips of the session, counting the
// open and the close.
func (c srepCost) RoundTrips() float64 { return 4 + c.Mores }

// sessionCost returns the expected cost of a stream of a difference of d IDs
// whose first batch is FirstBatch(e), of a responder holding count elements,
// with capacity capR.
func (p srepParams) sessionCost(tb testing.TB, table *srepDecodeTable, w *srepWire, d, e, count, capR int) srepCost {
	tb.Helper()
	var c srepCost
	ks := table.thresholds(tb, d)
	n0 := p.firstBatch(e)
	for _, k := range ks {
		st := p.stream(n0, k, capR)
		c.Symbols += float64(st.symbols)
		c.Mores += float64(st.mores())
		c.MoreShares[min(st.mores(), 3)]++
		if !st.decoded {
			c.Undecoded++
		}
		start := 0
		for i, b := range st.batches {
			if i > 0 {
				c.MoreBytes += float64(w.more(start + b))
			}
			c.SymbolBytes += float64(w.symbols(count, start, b))
			start += b
		}
	}
	kn := float64(len(ks))
	c.Symbols, c.Mores, c.SymbolBytes, c.MoreBytes, c.Undecoded = c.Symbols/kn, c.Mores/kn, c.SymbolBytes/kn, c.MoreBytes/kn, c.Undecoded/kn
	for i := range c.MoreShares {
		c.MoreShares[i] /= kn
	}
	return c
}

// srepIdeal is the closed form of §7.8 with the real decoder's costs: every
// round of MinRoundInterval, every pair reconciles once on the round's
// snapshot, so each upload is a difference of exactly its p(N − p) (holder,
// lacker) pairs.
type srepIdeal struct {
	Nodes int
	// DiffPerSession is the mean n_Δ of a pair per round and DiffPerUpload
	// n_Δ per upload.
	DiffPerSession, DiffPerUpload float64
	// SymbolsPerDiff is the symbols per difference with the constants, and
	// OracleSymbolsPerDiff with a responder that sends exactly the symbols
	// the decoder needs (§4.3.7's per-difference cost).
	SymbolsPerDiff, OracleSymbolsPerDiff float64
	// MoresPerSession and RoundTrips (open and close counted) per session
	// with a difference.
	MoresPerSession, RoundTrips float64
	// Bytes holds the metadata bytes per upload by kind.
	Bytes [srepKinds]float64
}

// MetadataPerUpload returns the metadata bytes per upload.
func (id srepIdeal) MetadataPerUpload() float64 {
	total := 0.0
	for _, b := range id.Bytes {
		total += b
	}
	return total
}

// Share returns the metadata per upload as a share of AC-B1's pullsync
// baseline.
func (id srepIdeal) Share() float64 { return id.MetadataPerUpload() / srepPullsyncACB1(id.Nodes) }

// OracleShare returns the share of the oracle, whose streams carry
// OracleSymbolsPerDiff symbols per difference in one batch: the floor that
// no first-batch or growth constant can go below.
func (id srepIdeal) OracleShare() float64 {
	m := id.MetadataPerUpload() - id.Bytes[srepMoreBytes] - id.Bytes[srepSymbolBytes]*(1-id.OracleSymbolsPerDiff/id.SymbolsPerDiff)
	return m / srepPullsyncACB1(id.Nodes)
}

func (id srepIdeal) String() string {
	parts := ""
	for k, b := range id.Bytes {
		parts += fmt.Sprintf(" %s %.0f", srepKindNames[k], b)
	}
	return fmt.Sprintf("ideal: metadata %.0f B per upload (%.0f%% of AC-B1's pullsync, the oracle's %.0f%%; per upload:%s); n_Δ %.1f per pair and round, %.1f per upload; "+
		"%.2f symbols per difference (oracle %.2f), %.2f Mores and %.2f round trips per session",
		id.MetadataPerUpload(), 100*id.Share(), 100*id.OracleShare(), parts, id.DiffPerSession, id.DiffPerUpload,
		id.SymbolsPerDiff, id.OracleSymbolsPerDiff, id.MoresPerSession, id.RoundTrips)
}

// srepBinomial returns the probabilities of 0 … n successes in n trials of
// probability q.
func srepBinomial(n int, q float64) []float64 {
	pmf := make([]float64, n+1)
	for k := range pmf {
		lg, _ := math.Lgamma(float64(n + 1))
		l1, _ := math.Lgamma(float64(k + 1))
		l2, _ := math.Lgamma(float64(n - k + 1))
		pmf[k] = math.Exp(lg - l1 - l2 + float64(k)*math.Log(q) + float64(n-k)*math.Log1p(-q))
	}
	return pmf
}

// ideal returns the closed form for perRound uploads per round. A pair's
// difference d is binomial (each upload is held by exactly one of the two
// with q = 2p(N − p) / (N(N − 1))) and splits binomially between the two
// sides, so the count estimate is |2X − d|; the hint is the previous
// round's difference, independent of this one. The session's cost is
// averaged over the decoder's thresholds of d. Every node's root changes in
// every round, so every pair exchanges a Status per round; each upload is
// fetched once by each of its N − p lackers, and every ordered pair with a
// difference opens a fetch (an upper bound on the Want framing).
func (p srepParams) ideal(tb testing.TB, table *srepDecodeTable, perRound float64) srepIdeal {
	tb.Helper()
	p = p.withDefaults()
	n, rep := p.Nodes, p.Replication
	u := int(math.Round(perRound))
	q := 2 * float64(rep*(n-rep)) / float64(n*(n-1))
	pd := srepBinomial(u, q)
	cdfD := make([]float64, len(pd))
	acc := 0.0
	for k, v := range pd {
		acc += v
		cdfD[k] = acc
	}
	count := p.Base + u
	capR := srepCapacity(count)
	w := newSREPWire(p.Root, p.LeafBits)

	var symbols, oracle, diff, mores, items, symBytes, moreBytes, pDiff float64
	for d, prob := range pd {
		if prob < 1e-12 || d == 0 {
			continue
		}
		pDiff += prob
		diff += prob * float64(d)
		oracle += prob * table.meanThreshold(tb, d)
		// the count estimate S = |2X − d|, X binomial(d, 1/2)
		cdfS := make([]float64, d+1)
		for x, px := range srepBinomial(d, 0.5) {
			cdfS[max(2*x-d, d-2*x)] += px
		}
		for e := 1; e <= d; e++ {
			cdfS[e] += cdfS[e-1]
		}
		cdf := func(e int) float64 {
			cs := cdfS[min(e, d)]
			if p.NoHints {
				return cs
			}
			return cs * cdfD[min(e, len(cdfD)-1)]
		}
		maxE := d
		if !p.NoHints {
			maxE = max(d, len(cdfD)-1)
		}
		for e := 1; e <= maxE; e++ {
			pe := cdf(e)
			if e > 1 {
				pe -= cdf(e - 1)
			}
			if pe < 1e-12 {
				continue
			}
			c := p.sessionCost(tb, table, w, d, e, count, capR)
			symbols += prob * pe * c.Symbols
			mores += prob * pe * c.Mores
			symBytes += prob * pe * c.SymbolBytes
			moreBytes += prob * pe * c.MoreBytes
		}
		var it float64
		for x, px := range srepBinomial(d, 0.5) {
			it += px * float64(w.items(x))
		}
		items += prob * it
	}
	pairs := float64(n*(n-1)) / 2
	perUpload := pairs / float64(u)
	id := srepIdeal{
		Nodes:                n,
		DiffPerSession:       diff,
		DiffPerUpload:        diff * perUpload,
		SymbolsPerDiff:       symbols / diff,
		OracleSymbolsPerDiff: oracle / diff,
		MoresPerSession:      mores / pDiff,
		RoundTrips:           4 + mores/pDiff,
	}
	id.Bytes[srepStatusBytes] = float64(n*(n-1)*w.status(count)) / float64(u)
	id.Bytes[srepSymbolBytes] = symBytes * perUpload
	id.Bytes[srepMoreBytes] = moreBytes * perUpload
	id.Bytes[srepItemBytes] = items * perUpload
	// every session, equal or not, has one opening and one empty Message
	id.Bytes[srepFPBytes] = float64(w.fp(count)) * perUpload
	id.Bytes[srepEmptyBytes] = float64(w.empty()) * perUpload
	// each lacker's ID, and each fetch's own bytes
	perID := w.want(2) - w.want(1)
	fetches := float64(n*(n-1)) * pDiff
	id.Bytes[srepWantBytes] = float64((n-rep)*perID) + fetches*float64(w.want(1)-perID)/float64(u)
	return id
}

// The pullsync baselines (§2.3–2.4). Every node offers each chunk it stores
// to each of its N − 1 peers (their live workers tail its bins), so an
// upload that reaches all N nodes is offered N(N − 1) times, whoever holds
// it already.
const (
	// srepPullsyncOffer is AC-B1's (and the harness's) bytes per offer:
	// one pb.Chunk (address, batch ID and stamp hash, 3 × (2 + 32) B).
	srepPullsyncOffer = 102
	// srepPullsyncEntry is an Offer entry on the wire: the pb.Chunk and its
	// tag and length in the Offer.
	srepPullsyncEntry = srepPullsyncOffer + 2
	// srepPullsyncPage is a page's own bytes: its Get (Bin, Start; ~9 B
	// delimited), the Offer's Topmost and length (~7 B) and the Want's bit
	// vector (~4 B for a page of up to 8 offers).
	srepPullsyncPage = 20
	// srepPageTimeout is how long a live page waits for the next chunk of
	// its bin (pullsync pageTimeout).
	srepPageTimeout = 250 * time.Millisecond
)

// srepPullsyncACB1 returns AC-B1's pullsync baseline per upload,
// bee_pullsync_chunks_offered × 102 B: N(N − 1) × 102 B.
func srepPullsyncACB1(n int) float64 { return float64(n*(n-1)) * srepPullsyncOffer }

// srepPullsync78 returns §7.8's pullsync figure per upload: N − 1 offers of
// 100 B to each of N nodes.
func srepPullsync78(n int) float64 { return float64(n*(n-1)) * 100 }

// srepPullsyncWire returns pullsync's metadata per upload on the wire at
// rate uploads per second: N(N − 1) Offer entries, and the page bytes of
// the pages they come in. A live page holds the chunks of its bin that
// arrive less than srepPageTimeout apart; with Poisson puts at rate μ into
// a bin that is e^(−μ·timeout) pages per chunk. Each node stores every
// upload once, so its bins from the storage radius on get rate/2, rate/4, …
func srepPullsyncWire(n int, rate float64) float64 {
	pages, share := 0.0, 0.5
	for range 31 {
		pages += share * math.Exp(-rate*share*srepPageTimeout.Seconds())
		share /= 2
	}
	return float64(n*(n-1)) * (srepPullsyncEntry + srepPullsyncPage*pages)
}
