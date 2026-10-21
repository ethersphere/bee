// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync_test

import (
	"bytes"
	"container/heap"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/p2p/streamtest"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	"github.com/ethersphere/bee/v2/pkg/storer"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"github.com/ethersphere/bee/v2/pkg/util/testutil/racedetection"
)

// The neighborhood simulation (proposal §13, T4.6): N nodes of one
// neighborhood (storage radius 0 unless simOptions.Radius sets one), each a
// rangesync.Service over a mock
// reserve with its reconciler.Reconciler, wired as pkg/node wires them and
// connected in a full mesh over streamtest. Uploads are pushed to the reserves
// of p random nodes (push replication), and rangesync spreads them to the
// others. runNeighborhood runs one simulation and returns what it measured
// (simResult).
//
// Time. The reconcilers and the services run on one fake clock (simClock).
// The harness moves it only while the neighborhood is quiet: every call of a
// reconciler into its service and every stream handler running or pending is
// parked (waiting to read a stream, or for a stream open's round trip with
// Delay), and nothing started for Settle of real time. It then moves the
// clock to the next pending timer. The work of one instant so finishes at that
// instant, and the measured times are the protocol's waits and, with Delay,
// its message legs, not the host's speed. A call waits for one of a peer's
// fetch slots before it counts as in flight (simNode.fetchSlot), so that it
// never waits for one inside the service while the fetch that holds the slot
// is parked. What runs on the real clock:
//   - the Index's root-change debounce. The reconciler's service view
//     (simService) replaces it: each node's root change is signaled
//     StatusMinInterval after the first change, on the fake clock, as the
//     Index does on the real one;
//   - the services' per-peer Status and session rate limits. They are lifted
//     (DisablePeerRateLimits), and the harness checks the same rates on the
//     fake clock (simResult.RateExceeded);
//   - the fetch rate limits. They are kept; a fetch that waits on them is a
//     call in flight, which holds the clock, so the wait takes no time on it.
//     simOptions.FetchClockRate adds a node-wide fetch rate on the clock;
//   - the fetch timeout, the session and Status timeouts, and the want set's
//     claim deadline and reject TTL. A read that waits for a stalled peer is
//     parked, so the clock moves on while they count real time: a stall
//     cannot be timed, and one of FetchTimeout of real time fails the fetch.
//
// Streams. streamtest routes each stream to the peer's protocol and runs its
// handler, but the bytes go through an unbounded in-test pipe (simPipe)
// instead of streamtest's records: those deadlock when a writer runs more
// than 16 writes ahead of its reader (TestFetchManyWants), which a fetch of
// more than 16 chunks does. The two ends of a stream are paired through a
// header that a Headler of the wrapped protocol echoes. A reset reaches the
// remote as a reset (errSimReset), not as the end of the stream, and a
// FullClose waits for the remote's close, as libp2p's does. With Delay,
// a stream open costs one round trip (the headers exchange), and every byte,
// close and reset reaches the other end Delay later; there is no bandwidth
// limit and no jitter.
//
// Rounds. A reconciler round follows the topology once (EachConnectedPeer),
// which nothing else does while the topology does not change, so simTopology
// records the time of every round of its node.

// simOptions configures a neighborhood simulation. Zero values select the
// defaults.
type simOptions struct {
	// Nodes is the neighborhood size N (default 5), and Replication is the
	// number p of random nodes each upload is pushed to (default 3).
	Nodes, Replication int
	// Seed seeds the overlays, the chunks, the upload targets and the
	// takeover jitters.
	Seed uint64
	// Base is the number of chunks every node holds at the start (default
	// 256). It keeps every node's count above reconciler.BootstrapRatio of
	// its peers' while uploads spread, so that no node bootstraps.
	Base int
	// Batches batches (at least one) of BatchSize uploads (defaults 4 and 8)
	// are pushed BatchInterval apart (default 17 s), the first Warmup after
	// the reconcilers start (default 61 s, by when the round interval has
	// grown to its maximum and a heartbeat has passed). The defaults keep
	// the uploads off the instants of the rounds and heartbeats of a quiet
	// neighborhood, which fall on multiples of 5 s.
	Batches, BatchSize    int
	BatchInterval, Warmup time.Duration
	// ChunkSize is the payload size of an upload (default swarm.ChunkSize).
	ChunkSize int
	// Tail is how long the simulation runs on after the last upload
	// converged (default 2 × MaxRoundInterval).
	Tail time.Duration
	// Limit bounds the simulated time after the last upload (default 10
	// minutes): a simulation that has not converged by then fails.
	Limit time.Duration
	// Settle is how long, in real time, the neighborhood must stay quiet
	// before the clock moves (default 15 ms, 10 times that under the race
	// detector).
	Settle time.Duration
	// Delay is the one-way latency of every stream, on the clock (default 0:
	// none): the bytes one end writes, the end of its writes and a reset
	// reach the other end Delay after they happened (simPipe), and a stream
	// open is a round trip. A message exchange then takes 2 × Delay, so the
	// run times calls by their legs (Spans): a Status send 4 × Delay (the
	// open, the Status, the handler's close), a session its open, a round
	// trip per pair of messages and its close's round trip (6 × Delay at the
	// least), and a fetch its open, Want and Delivery legs. The fetch rate
	// limits and the fetch timeout stay on the real clock.
	Delay time.Duration
	// NoHints runs every service without hint(R) (DisableHints): the first
	// symbol batch then comes from the count estimate alone, as for G6's
	// sweep with the hint on and off.
	NoHints bool
	// Lag base chunks, the last Lag of Base, are missing from node LagNode
	// (an index into simNeighborhood's nodes) at the start: the node rejoins
	// after an absence and catches up, in a session past CatchUpMinDiff, a
	// split past a cache, or a bootstrap (Lag = Base). With Lag set, the
	// counters are taken from the start of the run instead of from the first
	// upload, and the run converges once the node holds those chunks too
	// (simResult.CaughtUp).
	Lag, LagNode int
	// FetchOpenFails lists nodes (indexes into simNeighborhood's nodes) to
	// which every fetch stream open fails (errSimOpenRefused): a peer that
	// lists its IDs in sessions and never serves a Want.
	FetchOpenFails []int
	// MinRoundInterval and MaxRoundInterval override the bounds of the
	// adaptive round interval (reconciler.Options); zero keeps
	// reconciler.MinRoundInterval and MaxRoundInterval.
	MinRoundInterval, MaxRoundInterval time.Duration
	// Reconciler and Service, if set, adjust each node's options after the
	// harness set its own (clock, jitter, bounds, LargeDiff; leaf bits 8,
	// scorer, wants, striper, clock, LargeDiff).
	Reconciler func(*reconciler.Options)
	Service    func(*rangesync.Options)
	// LargeDiff scales the threshold of the sessions' large-difference
	// branch and of the reconcilers' session-root check (Options.LargeDiff
	// of both); zero keeps rangesync.LargeDiff (T4.7 part 1).
	LargeDiff uint64
	// Radius sets the storage radius of nodes by index (default 0): a
	// node's root is the first Radius bits of its overlay, and a chunk
	// converges once every node whose root contains it holds it. Overlay,
	// if set, adjusts the random overlay of node i, for example to place
	// nodes in one root.
	Radius  map[int]uint8
	Overlay func(i int, a swarm.Address) swarm.Address
	// ChunkRoot, if set (Bits > 0), holds every generated chunk, base and
	// uploads; LagRoot, if set, holds the lagging base chunks instead (for
	// example one level-1 range: a deficit in a child).
	ChunkRoot, LagRoot fingerprint.Range
	// FetchClockRate, if set, is a node-wide fetch rate on the clock, in
	// chunks per second: every Want's IDs (MaxWantIDs at most) take their
	// share of it before they are requested, as the service's fetch limiter
	// does on the real clock (simService.fetchOnClock).
	FetchClockRate int
}

// steadyOptions returns the options of a steady-state run of nodes nodes
// with p = 3: 24 batches of 20 uploads 1.3 s apart, about 15 uploads per
// second, so that sessions see the load of a steady stream rather than of
// separate bursts (AC-B1's reference load is a steady rate).
func steadyOptions(nodes int, seed uint64) simOptions {
	return simOptions{Nodes: nodes, Replication: 3, Seed: seed, Batches: 24, BatchSize: 20, BatchInterval: 1300 * time.Millisecond}
}

func (o simOptions) withDefaults() simOptions {
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
	def(&o.Nodes, 5)
	def(&o.Replication, 3)
	def(&o.Base, 256)
	def(&o.Batches, 4)
	def(&o.BatchSize, 8)
	def(&o.ChunkSize, swarm.ChunkSize)
	defDur(&o.BatchInterval, 17*time.Second)
	defDur(&o.Warmup, 61*time.Second)
	defDur(&o.Tail, 2*reconciler.MaxRoundInterval)
	defDur(&o.Limit, 10*time.Minute)
	settle := 15 * time.Millisecond
	if racedetection.On {
		settle *= 10 // goroutines run slower: a clock moved early delays a convergence
	}
	defDur(&o.Settle, settle)
	return o
}

// simResult is what a simulation measured. Counters of the nodes are taken
// from the first upload to the end of the run (the warm-up is left out), or
// from the start with a lagging node (simOptions.Lag): "from the first upload"
// below means from then.
type simResult struct {
	// Batches holds each upload batch's convergence.
	Batches []simBatch
	// Nodes holds each node's counters, in the order of simNeighborhood's
	// nodes.
	Nodes []simNodeResult
	// PairwiseDiff holds every pairwise_diff sample of the run, warm-up
	// included, in the order observed, with what the side reported
	// (rangesync.SessionSample): each completed session is observed by both
	// of its sides.
	PairwiseDiff []simSample
	// Sessions counts the sessions the reconcilers opened, by result
	// (reconciler sessions_total), from the first upload.
	Sessions map[string]float64
	// Counters holds, summed over the nodes and from the first upload, the
	// counters of simCounterNames by their labels, keyed as
	// "streams_exhausted_total{level=root}". With a lagging node,
	// CatchUpCounters holds the part of them counted before the first upload
	// (the catch-up's, once it ended by then).
	Counters, CatchUpCounters map[string]float64
	// StreamBytes holds the bytes written on the streams opened from the
	// first upload on, both directions, by stream name, and TailBytes those
	// of the streams opened after the last upload converged.
	StreamBytes, TailBytes map[string]int
	// Flows holds the bytes of StreamBytes by direction: what one node
	// wrote to another on the streams of one name that one of them opened.
	Flows map[simFlow]int
	// Spans holds every call of a reconciler into its service (a session it
	// opened, a fetch, a Status sent), warm-up included, in the order they
	// ended.
	Spans []simSpan
	// Duplicates counts the rangesync deliveries of a chunk to a node that
	// had received it from rangesync before.
	Duplicates int
	// Protocols holds the protocol of every stream a node opened.
	Protocols map[string]bool
	// RateExceeded counts the Statuses and the sessions that a peer's
	// inbound limit (PeerStatusInterval and PeerStatusBurst,
	// PeerSessionInterval and PeerSessionBurst) would have refused on the
	// fake clock.
	RateExceeded int
	// Converged is the time from the first upload to the convergence of
	// the last upload, and LastSession the time from then to the start of
	// the last session a reconciler opened (0 if none started after it).
	Converged, LastSession time.Duration
	// CaughtUp is the time from the start of the run until the lagging node
	// (simOptions.Lag) held every base chunk, 0 without one.
	CaughtUp time.Duration
	// End is the time from the start of the run to its end.
	End time.Duration
	// Steps is the number of times the clock moved, and Wall the real time
	// of the run.
	Steps int
	Wall  time.Duration
}

// simFlow keys simResult.Flows: node From wrote the bytes and node To read
// them (indexes into simNeighborhood's nodes) on streams named Stream that
// node Opener, one of the two, opened.
type simFlow struct {
	From, To, Opener int
	Stream           string
}

// simBatch is the convergence of one upload batch.
type simBatch struct {
	// At is the time of the upload after the start.
	At time.Duration
	// Convergence is the time from the upload until every node held every
	// chunk of the batch, and Rounds the most rounds any node ran in that
	// time, both instants included: the uploads are scheduled before the
	// reconcilers start, so at the instant of an upload it comes first.
	Convergence time.Duration
	Rounds      int
}

// simNodeResult is one node's counters.
type simNodeResult struct {
	Addr swarm.Address
	// Uploads counts the chunks pushed to the node, Stored the chunks
	// rangesync stored in its reserve, and Deliveries the rangesync
	// Deliveries it received.
	Uploads, Stored, Deliveries int
	// Rounds counts the node's rounds in the whole run, RoundGap is the
	// shortest time between two of them (0 with fewer than two), and
	// RoundInterval is its round interval at the end
	// (round_interval_seconds).
	Rounds        int
	RoundGap      time.Duration
	RoundInterval time.Duration
	// MetadataIn and MetadataOut are bytes_metadata{in,out}, ChunksIn and
	// ChunksOut bytes_chunks{in,out}, and MoreRequests more_requests_total.
	MetadataIn, MetadataOut, ChunksIn, ChunksOut, MoreRequests float64
	// WantClaims holds want_claims by result.
	WantClaims map[string]float64
	// PairwiseDiff is the sample count of the pairwise_diff histogram that
	// the node exposes (Service.Metrics).
	PairwiseDiff uint64
}

// simSpan is one call of a reconciler into its service, on the clock.
type simSpan struct {
	Kind string // "reconcile", "fetch" or "status"
	Node int
	Peer swarm.Address
	// Stripe marks a session of a striped transfer or of its verification:
	// opened with ranges (Open) and without a Striper.
	Stripe bool
	// At is when the reconciler made the call, after the start of the run;
	// Wait is how long a fetch waited for one of the peer's fetch slots
	// (simNode.fetchSlot) before the service ran it, and Duration how long
	// the service then took: with simOptions.Delay, its message legs (a
	// session takes its open, a round trip per pair of messages and the
	// FullClose's round trip).
	At, Wait, Duration time.Duration
	// Streams counts the streams the call opened and Stored the chunks a
	// fetch stored. FirstDelivery is the time from the service's start to
	// the arrival of a fetch's first Delivery, 0 without one: with Delay
	// the open, the Want and the Delivery legs. All Deliveries of a Want
	// arrive in one instant (there is no bandwidth limit, and the holder's
	// delivery limiter runs on the real clock), so the first is the last.
	Streams, Stored int
	FirstDelivery   time.Duration
	Err             error
}

// simCallKey is the context key of the simCall of a reconciler's call into
// its service; the service passes the context on to the streams it opens.
type simCallKey struct{}

// simCall collects the streams one call opened.
type simCall struct {
	mu      sync.Mutex
	streams []*simPair
	stripe  bool // a stripe session (simSpan.Stripe), set before the call
}

// simSample is one pairwise_diff sample: when and by which node it was
// observed, and what the node's side of the session reported.
type simSample struct {
	At      time.Duration // after the start
	Node    int
	Diff    float64
	Session rangesync.SessionSample
	// Wants is the number of IDs the side handed to its reconciler, the
	// value of pairwise_diff_half and what the reconciler compares with
	// CatchUpMinDiff: the wants ReconcileWith returned to an initiator, and
	// those a responder passed to Options.Wants (0 when it passed none).
	Wants int
}

// simSampleKey is the side of a session whose sample waits for its wants.
type simSampleKey struct {
	node      int
	peer      string
	initiator bool
}

// wantClaimResults are the label values of want_claims.
var wantClaimResults = []string{"granted", "denied", "peer_full", "full", "rejected", "closed", "expired", "late"}

// runNeighborhood runs one neighborhood simulation with o and returns its
// measurements. It fails tb if the uploads do not converge within o.Limit.
func runNeighborhood(tb testing.TB, o simOptions) *simResult {
	tb.Helper()
	return newNeighborhood(tb, o).run()
}

// simNeighborhood is a running simulation.
type simNeighborhood struct {
	tb    testing.TB
	o     simOptions
	clock *simClock
	rng   *rand.Rand // the harness goroutine's only
	nodes []*simNode
	roots []fingerprint.Range // of the nodes, by index
	start time.Time

	// inflight counts the calls of reconcilers into their services and the
	// stream handlers running or about to run, and parked those of their
	// goroutines that wait for a stream or the clock (simPipe, sleep);
	// activity is bumped by every event that may start work.
	inflight atomic.Int64
	parked   atomic.Int64
	activity atomic.Uint64

	pairSeq atomic.Uint64
	pairs   sync.Map // pair id → *simPair, until its handler takes it

	closing atomic.Bool // close was called

	mu        sync.Mutex
	chunks    map[string]*simChunk // uploads and lagging base chunks by ID
	uploads   []*simChunk
	lagging   []*simChunk // the base chunks the lagging node lacks
	batchAt   []time.Time
	samples   []simSample
	awaiting  map[simSampleKey]int // samples waiting for their wants, by side
	protocols map[string]bool
	arrivals  map[simArrival][]time.Time
	sessions  []time.Time // starts of the sessions the reconcilers opened
	streams   []*simPair
	closed    bool // close closed the streams: NewStream opens no more
	spans     []simSpan
	converged int       // uploads and lagging chunks held by every node
	from      time.Time // the counters are taken from then (snapshot)
	baseline  []simCounters
	atUpload  map[string]float64 // with a lagging node: Counters at the first upload
	sessions0 map[string]float64
}

// simChunk is one upload.
type simChunk struct {
	batch int
	at    time.Time
	// holders counts the nodes holding it whose root contains it, and want
	// the nodes whose root contains it.
	holders, want int
	converged     time.Time
}

// simArrival keys the Statuses or sessions a node received from a peer.
type simArrival struct {
	to, from string
	status   bool
}

// simCounters are a node's metrics at one time.
type simCounters struct {
	metaIn, metaOut, chunksIn, chunksOut, mores float64
	claims                                      map[string]float64
	diffs                                       uint64
	labeled                                     map[string]float64 // simCounterNames
}

// simCounterNames are the labeled counters that simResult.Counters holds:
// the service's (stream exhaustion by level, parts sent by type, fetches
// skipped by reason, Busy answers) and the reconciler's (stripes by result).
var simCounterNames = struct{ service, reconciler []string }{
	service:    []string{"streams_exhausted_total", "parts_sent", "fetch_skipped_total", "session_busy_total"},
	reconciler: []string{"stripes_total"},
}

// simNode is one node of the neighborhood.
type simNode struct {
	nb     *simNeighborhood
	i      int // index in nb.nodes
	addr   swarm.Address
	store  *mockstorer.ReserveStore
	rec    *streamtest.RecorderDisconnecter
	svc    *rangesync.Service
	r      *reconciler.Reconciler
	spec   p2p.ProtocolSpec // svc's protocol, wrapped for the harness
	routes map[string]p2p.ProtocolSpec
	root   chan struct{} // the reconciler's root-change signal

	rootRange fingerprint.Range // the first simOptions.Radius bits of addr

	fetchMu    sync.Mutex
	fetchSlots map[string]chan struct{} // by peer
	fetchFree  time.Time                // simOptions.FetchClockRate: when its share is free

	// guarded by nb.mu
	held       map[string]time.Time // IDs stored, with the time
	puts       int
	uploads    int
	rounds     []time.Time
	debouncing bool
}

func newNeighborhood(tb testing.TB, o simOptions) *simNeighborhood {
	tb.Helper()
	o = o.withDefaults()
	if o.Replication > o.Nodes {
		tb.Fatalf("replication %d over %d nodes", o.Replication, o.Nodes)
	}
	if o.Lag < 0 || o.Lag > o.Base || o.LagNode < 0 || o.LagNode >= o.Nodes {
		tb.Fatalf("lag %d of %d base chunks at node %d of %d", o.Lag, o.Base, o.LagNode, o.Nodes)
	}
	nb := &simNeighborhood{
		tb:        tb,
		o:         o,
		rng:       rand.New(rand.NewPCG(o.Seed, o.Seed^0x9e3779b97f4a7c15)),
		chunks:    make(map[string]*simChunk),
		awaiting:  make(map[simSampleKey]int),
		protocols: make(map[string]bool),
		arrivals:  make(map[simArrival][]time.Time),
	}
	nb.clock = &simClock{now: time.Unix(1_000_000, 0), touch: nb.touch}
	base := make([]swarm.Chunk, o.Base)
	for i := range base {
		base[i] = swarm.NewChunk(inRange(nb.randAddr(), o.ChunkRoot), []byte{1}).WithStamp(nb.newStamp())
	}
	// the lagging node fetches the chunks it lacks, so they must pass the
	// content check: valid content-addressed chunks (the other base chunks
	// are never fetched, and runs without a lag keep their chunks)
	lagRoot := o.LagRoot
	if lagRoot.Bits == 0 {
		lagRoot = o.ChunkRoot
	}
	for i := o.Base - o.Lag; i < o.Base; i++ {
		base[i] = nb.newCAC(8, lagRoot).WithStamp(nb.newStamp())
	}
	addrs := make([]swarm.Address, 0, o.Nodes)
	for len(addrs) < o.Nodes {
		a := nb.randAddr()
		if o.Overlay != nil {
			a = o.Overlay(len(addrs), a)
		}
		if !slices.ContainsFunc(addrs, a.Equal) {
			addrs = append(addrs, a)
		}
	}
	for i, a := range addrs {
		nb.roots = append(nb.roots, fingerprint.NewRange(a, o.Radius[i]))
	}
	for _, ch := range base[o.Base-o.Lag:] {
		key, err := chunkKey(ch)
		if err != nil {
			tb.Fatal(err)
		}
		c := &simChunk{batch: -1, at: nb.clock.Now(), want: nb.want(ch.Address())}
		c.holders = c.want
		if nb.roots[o.LagNode].Contains(ch.Address()) {
			c.holders--
		}
		if c.holders == c.want { // outside the lagging node's root
			c.converged = c.at
			nb.converged++
		}
		nb.chunks[key] = c
		nb.lagging = append(nb.lagging, c)
	}
	tb.Cleanup(nb.close)
	for i, a := range addrs {
		held := base
		if i == o.LagNode {
			held = base[:o.Base-o.Lag]
		}
		nb.nodes = append(nb.nodes, nb.newNode(i, a, addrs, held))
	}
	for _, n := range nb.nodes {
		for _, p := range nb.nodes {
			if p != n {
				n.routes[p.addr.String()] = p.spec
			}
		}
	}
	// Every node holds every peer's Status before the reconcilers start, as
	// in roundNode.start: the first round's sessions would otherwise meet a
	// peer that has not stored the Status yet. With Delay, the sends wait on
	// the clock, which moves until they arrived; no other timer is pending
	// yet.
	var sends sync.WaitGroup
	errs := make(chan error, o.Nodes*o.Nodes)
	for _, n := range nb.nodes {
		for _, p := range nb.nodes {
			if p != n {
				done := nb.call()
				sends.Go(func() {
					defer done()
					errs <- n.svc.SendStatus(context.Background(), p.addr)
				})
			}
		}
	}
	for nb.waitQuiet(); ; nb.waitQuiet() {
		next, ok := nb.clock.next()
		if !ok {
			break
		}
		nb.clock.advance(next)
	}
	sends.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			tb.Fatal(err)
		}
	}
	for _, n := range nb.nodes {
		for _, p := range nb.nodes {
			if _, ok := n.svc.LastStatus(p.addr); p != n && !ok {
				tb.Fatalf("node %s has no Status of %s", n.addr, p.addr)
			}
		}
	}
	return nb
}

func (nb *simNeighborhood) newNode(i int, a swarm.Address, all []swarm.Address, base []swarm.Chunk) *simNode {
	tb, o := nb.tb, nb.o
	n := &simNode{
		nb:         nb,
		i:          i,
		addr:       a,
		routes:     make(map[string]p2p.ProtocolSpec),
		root:       make(chan struct{}, 1),
		fetchSlots: make(map[string]chan struct{}),
		held:       make(map[string]time.Time),
	}
	n.rootRange = fingerprint.NewRange(a, o.Radius[i])
	n.store = mockstorer.NewReserve(mockstorer.WithAddrIndex(), mockstorer.WithChunks(base...), mockstorer.WithPutHook(n.putHook), mockstorer.WithRadius(o.Radius[i]))
	n.rec = streamtest.NewRecorderDisconnecter(streamtest.New(streamtest.WithBaseAddr(a), streamtest.WithPeerProtocols(n.routes)))

	var peers []swarm.Address
	for _, p := range all {
		if !p.Equal(a) {
			peers = append(peers, p)
		}
	}
	var jitterMu sync.Mutex
	jitter := rand.New(rand.NewPCG(o.Seed, uint64(i)))
	ro := reconciler.Options{
		Now:   nb.clock.Now,
		After: nb.clock.After,
		Jitter: func() float64 {
			jitterMu.Lock()
			defer jitterMu.Unlock()
			return jitter.Float64()
		},
		MinRoundInterval: o.MinRoundInterval,
		MaxRoundInterval: o.MaxRoundInterval,
		LargeDiff:        o.LargeDiff,
	}
	if o.Reconciler != nil {
		o.Reconciler(&ro)
	}
	n.r = reconciler.New(a, &simTopology{n: n, peers: peers}, nil, log.Noop, ro)

	wants := func(peer swarm.Address, ids []storer.ChunkID) {
		nb.setWants(n, peer, false, len(ids))
		n.r.Wants(peer, ids)
	}
	// as pkg/node wires them (rangesyncOptions), the Striper included
	so := rangesync.Options{LeafBits: serviceLeafBits, Scorer: n.r, Wants: wants, Striper: n.r, Now: nb.clock.Now, LargeDiff: o.LargeDiff}
	if o.Service != nil {
		o.Service(&so)
	}
	accept := func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }
	n.svc = rangesync.New(&simStreamer{StreamerDisconnecter: n.rec, n: n}, n.store, a, nil, nil, accept, log.Noop, so)
	n.svc.DisablePeerRateLimits()
	if o.NoHints {
		n.svc.DisableHints()
	}
	n.svc.OnSession(func(ss rangesync.SessionSample) { nb.sample(n, ss) })
	n.r.SetService(&simService{Service: n.svc, n: n})
	if err := n.svc.Start(); err != nil {
		tb.Fatal(err)
	}
	waitFor(tb, "index", func() bool {
		_, err := n.svc.LocalStatus()
		return err == nil
	})
	n.spec = n.wrapProtocol()
	return n
}

// close closes every stream and refuses new ones, then stops the reconcilers,
// then the services. Closing the streams first ends the reads and the
// FullCloses still waiting on one: what they wait for (bytes, the remote's
// close, simCloseDeadline) is due on the clock, which no longer moves, and
// Reconciler.Close waits for its calls to end. A FullClose then returns as on
// the remote's close (simPipe.awaitEOF), and a read returns io.EOF after the
// bytes that arrived. Only the first call closes: a later one returns at once,
// also while the first still waits (the cleanup of a test that gave up on a
// stuck teardown would otherwise wait for it too).
func (nb *simNeighborhood) close() {
	if !nb.closing.CompareAndSwap(false, true) {
		return
	}
	nb.mu.Lock()
	nb.closed = true
	streams := slices.Clone(nb.streams)
	nb.mu.Unlock()
	for _, s := range streams {
		s.toHandler.closeRead(io.EOF, streamtest.ErrStreamClosed)
		s.toInitiator.closeRead(io.EOF, streamtest.ErrStreamClosed)
	}
	for _, n := range nb.nodes {
		if err := n.r.Close(); err != nil {
			nb.tb.Error(err)
		}
	}
	for _, n := range nb.nodes {
		if err := n.svc.Close(); err != nil {
			nb.tb.Error(err)
		}
	}
}

func (nb *simNeighborhood) touch() { nb.activity.Add(1) }

// call marks a call of a reconciler into its service; the returned function
// ends it.
func (nb *simNeighborhood) call() (done func()) {
	nb.inflight.Add(1)
	nb.touch()
	return func() {
		nb.touch()
		nb.inflight.Add(-1)
	}
}

// sleep waits d on the clock, parked like a reader of a stream: not counted
// as work in flight. It returns ctx's error if ctx ends first.
func (nb *simNeighborhood) sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	var once sync.Once
	unpark := func() {
		once.Do(func() {
			nb.parked.Add(-1)
			nb.touch()
		})
	}
	woken := make(chan struct{})
	nb.parked.Add(1)
	nb.clock.AfterFunc(d, func() {
		unpark()
		close(woken)
	})
	select {
	case <-woken:
		return nil
	case <-ctx.Done():
		unpark()
		return ctx.Err()
	}
}

// waitQuiet waits until every call and handler in flight is parked (a read
// of a stream, a sleep) and nothing started for Settle.
func (nb *simNeighborhood) waitQuiet() {
	const poll = time.Millisecond
	deadline := time.Now().Add(2 * time.Minute)
	last, since := nb.activity.Load(), time.Now()
	for {
		time.Sleep(poll)
		if a := nb.activity.Load(); a != last || nb.inflight.Load() > nb.parked.Load() {
			last, since = a, time.Now()
		} else if time.Since(since) >= nb.o.Settle {
			return
		}
		if time.Now().After(deadline) {
			nb.tb.Fatalf("neighborhood not quiet: %d calls in flight, %d parked, at %v", nb.inflight.Load(), nb.parked.Load(), nb.clock.Now().Sub(nb.start))
		}
	}
}

// run starts the reconcilers, pushes the uploads on schedule and moves the
// clock from one pending timer to the next until every upload converged and
// the tail passed.
func (nb *simNeighborhood) run() *simResult {
	o := nb.o
	wall := time.Now()
	nb.start = nb.clock.Now()
	for b := range o.Batches {
		nb.clock.AfterFunc(o.Warmup+time.Duration(b)*o.BatchInterval, func() { nb.upload(b) })
	}
	if o.Lag > 0 {
		nb.snapshot() // the catch-up starts with the first round
	}
	for _, n := range nb.nodes {
		n.r.Start(context.Background())
	}
	limit := nb.start.Add(o.Warmup + time.Duration(o.Batches-1)*o.BatchInterval + o.Limit)
	var done time.Time
	for steps := 0; ; steps++ {
		nb.waitQuiet()
		if done.IsZero() && nb.allConverged() {
			done = nb.clock.Now()
		}
		next, ok := nb.clock.next()
		switch end := done.Add(o.Tail); {
		case !done.IsZero() && (!ok || next.After(end)):
			nb.clock.advance(end)
			nb.waitQuiet()
			return nb.result(done, steps, time.Since(wall))
		case !ok:
			nb.tb.Fatal("no timer pending")
		case done.IsZero() && next.After(limit): // a long Tail may pass it once converged
			nb.tb.Fatalf("uploads not converged %v after the last one: %s", o.Limit, nb.progress())
		}
		nb.clock.advance(next)
	}
}

// allConverged reports whether every batch was uploaded and every node holds
// every upload and every base chunk.
func (nb *simNeighborhood) allConverged() bool {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	return len(nb.batchAt) == nb.o.Batches && nb.converged == len(nb.uploads)+len(nb.lagging)
}

// progress describes the uploads that have not converged.
func (nb *simNeighborhood) progress() string {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	missing := 0
	for _, c := range nb.uploads {
		if c.holders < c.want {
			missing++
		}
	}
	lagging := 0
	for _, c := range nb.lagging {
		if c.holders < c.want {
			lagging++
		}
	}
	held := make([]string, len(nb.nodes))
	for i, n := range nb.nodes {
		held[i] = strconv.Itoa(len(n.held))
	}
	return fmt.Sprintf("%d of %d uploads (%d batches) and %d of %d lagging base chunks not held by every node; IDs stored per node: %s",
		missing, len(nb.uploads), len(nb.batchAt), lagging, len(nb.lagging), strings.Join(held, " "))
}

// upload pushes batch b: BatchSize new chunks, each to Replication random
// nodes.
func (nb *simNeighborhood) upload(b int) {
	switch {
	case b == 0 && nb.o.Lag == 0:
		nb.snapshot()
	case b == 0:
		nb.atUpload = nb.labeledSince()
	}
	now := nb.clock.Now()
	nb.mu.Lock()
	nb.batchAt = append(nb.batchAt, now)
	nb.mu.Unlock()
	for range nb.o.BatchSize {
		ch := nb.newChunk()
		key, err := chunkKey(ch)
		if err != nil {
			nb.tb.Fatal(err)
		}
		c := &simChunk{batch: b, at: now, want: nb.want(ch.Address())}
		nb.mu.Lock()
		nb.chunks[key] = c
		nb.uploads = append(nb.uploads, c)
		if c.want == 0 { // in no node's root
			c.converged = now
			nb.converged++
		}
		nb.mu.Unlock()
		for _, i := range nb.rng.Perm(len(nb.nodes))[:nb.o.Replication] {
			n := nb.nodes[i]
			nb.mu.Lock()
			n.uploads++
			nb.mu.Unlock()
			if err := n.store.ReservePutter().Put(context.Background(), ch); err != nil {
				nb.tb.Fatal(err)
			}
		}
	}
}

// labeledSince returns the labeled counters (simCounterNames) summed over
// the nodes since the snapshot.
func (nb *simNeighborhood) labeledSince() map[string]float64 {
	out := make(map[string]float64)
	for i, n := range nb.nodes {
		for k, v := range n.counters().labeled {
			out[k] += v - nb.baseline[i].labeled[k]
		}
	}
	return out
}

// snapshot records the counters at the first upload, or at the start with a
// lagging node.
func (nb *simNeighborhood) snapshot() {
	nb.mu.Lock()
	nb.from = nb.clock.Now()
	nb.mu.Unlock()
	nb.baseline = make([]simCounters, len(nb.nodes))
	for i, n := range nb.nodes {
		nb.baseline[i] = n.counters()
	}
	nb.sessions0 = nb.sessionResults()
}

func (n *simNode) counters() simCounters {
	c := simCounters{
		metaIn:    n.svc.MetadataBytes("in"),
		metaOut:   n.svc.MetadataBytes("out"),
		chunksIn:  n.svc.ChunkBytes("in"),
		chunksOut: n.svc.ChunkBytes("out"),
		mores:     n.svc.MoreRequests(),
		claims:    make(map[string]float64),
	}
	for _, r := range wantClaimResults {
		c.claims[r] = n.svc.WantClaims(r)
	}
	for _, m := range gathered(n.nb.tb, n.svc.Metrics(), "_pairwise_diff") {
		c.diffs += m.GetHistogram().GetSampleCount()
	}
	c.labeled = make(map[string]float64)
	for _, name := range simCounterNames.service {
		maps.Copy(c.labeled, labeledCounters(n.nb.tb, n.svc.Metrics(), name))
	}
	for _, name := range simCounterNames.reconciler {
		maps.Copy(c.labeled, labeledCounters(n.nb.tb, n.r.Metrics(), name))
	}
	return c
}

// labeledCounters returns the counters of cs named name (after the
// namespace), keyed by name and labels, as "streams_exhausted_total{level=root}".
func labeledCounters(tb testing.TB, cs []prometheus.Collector, name string) map[string]float64 {
	tb.Helper()
	out := make(map[string]float64)
	for _, m := range gathered(tb, cs, "_"+name) {
		labels := make([]string, 0, len(m.GetLabel()))
		for _, l := range m.GetLabel() {
			labels = append(labels, l.GetName()+"="+l.GetValue())
		}
		out[name+"{"+strings.Join(labels, ",")+"}"] += m.GetCounter().GetValue()
	}
	return out
}

// gathered returns the metrics of cs whose name ends with suffix, as a
// registry exposes them.
func gathered(tb testing.TB, cs []prometheus.Collector, suffix string) []*dto.Metric {
	tb.Helper()
	reg := prometheus.NewRegistry()
	reg.MustRegister(cs...)
	mfs, err := reg.Gather()
	if err != nil {
		tb.Fatal(err)
	}
	var out []*dto.Metric
	for _, mf := range mfs {
		if strings.HasSuffix(mf.GetName(), suffix) {
			out = append(out, mf.GetMetric()...)
		}
	}
	return out
}

// sessionResults returns reconciler sessions_total by result, summed over
// the nodes.
func (nb *simNeighborhood) sessionResults() map[string]float64 {
	out := make(map[string]float64)
	for _, n := range nb.nodes {
		for _, m := range gathered(nb.tb, n.r.Metrics(), "_sessions_total") {
			for _, l := range m.GetLabel() {
				if l.GetName() == "result" {
					out[l.GetValue()] += m.GetCounter().GetValue()
				}
			}
		}
	}
	return out
}

func (nb *simNeighborhood) sample(n *simNode, ss rangesync.SessionSample) {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	nb.samples = append(nb.samples, simSample{At: nb.clock.Now().Sub(nb.start), Node: n.i, Diff: float64(ss.Difference), Session: ss})
	nb.awaiting[simSampleKey{node: n.i, peer: ss.Peer.ByteString(), initiator: ss.Initiator}] = len(nb.samples) - 1
}

// setWants records the wants of node n's side of its last session with peer
// on its sample. The sample comes first (the service's session seam runs
// before it returns the wants or passes them on), and a node runs one
// session per peer and side at a time.
func (nb *simNeighborhood) setWants(n *simNode, peer swarm.Address, initiator bool, wants int) {
	key := simSampleKey{node: n.i, peer: peer.ByteString(), initiator: initiator}
	nb.mu.Lock()
	defer nb.mu.Unlock()
	if i, ok := nb.awaiting[key]; ok {
		nb.samples[i].Wants = wants
		delete(nb.awaiting, key)
	}
}

// arrived records a Status or session that node to received from peer from.
func (nb *simNeighborhood) arrived(to *simNode, from swarm.Address, stream string) {
	if stream != rangesync.StatusStreamName && stream != rangesync.ReconcileStreamName {
		return
	}
	k := simArrival{to: to.addr.ByteString(), from: from.ByteString(), status: stream == rangesync.StatusStreamName}
	now := nb.clock.Now()
	nb.mu.Lock()
	defer nb.mu.Unlock()
	nb.arrivals[k] = append(nb.arrivals[k], now)
}

// result collects the measurements of a run that converged at done.
func (nb *simNeighborhood) result(done time.Time, steps int, wall time.Duration) *simResult {
	sessions := nb.sessionResults()
	nb.mu.Lock()
	defer nb.mu.Unlock()
	res := &simResult{
		PairwiseDiff:    slices.Clone(nb.samples),
		Spans:           slices.Clone(nb.spans),
		Sessions:        make(map[string]float64),
		StreamBytes:     make(map[string]int),
		TailBytes:       make(map[string]int),
		Flows:           make(map[simFlow]int),
		Protocols:       maps.Clone(nb.protocols),
		Counters:        make(map[string]float64),
		CatchUpCounters: maps.Clone(nb.atUpload),
		Converged:       done.Sub(nb.batchAt[0]),
		End:             nb.clock.Now().Sub(nb.start),
		Steps:           steps,
		Wall:            wall,
	}
	for k, v := range sessions {
		res.Sessions[k] = v - nb.sessions0[k]
	}
	for b, at := range nb.batchAt {
		conv := at
		for _, c := range nb.uploads {
			if c.batch == b && c.converged.After(conv) {
				conv = c.converged
			}
		}
		rounds := 0
		for _, n := range nb.nodes {
			k := 0
			for _, r := range n.rounds {
				if !r.Before(at) && !r.After(conv) {
					k++
				}
			}
			rounds = max(rounds, k)
		}
		res.Batches = append(res.Batches, simBatch{At: at.Sub(nb.start), Convergence: conv.Sub(at), Rounds: rounds})
	}
	for _, at := range nb.sessions {
		res.LastSession = max(res.LastSession, at.Sub(done))
	}
	for _, c := range nb.lagging {
		res.CaughtUp = max(res.CaughtUp, c.converged.Sub(nb.start))
	}

	// rangesync deliveries, read off the fetch streams
	delivered := make(map[*simNode]map[string]int)
	for _, s := range nb.streams {
		if !s.at.Before(nb.from) {
			res.StreamBytes[s.stream] += s.toHandler.size() + s.toInitiator.size()
			if s.to != nil {
				res.Flows[simFlow{From: s.from.i, To: s.to.i, Opener: s.from.i, Stream: s.stream}] += s.toHandler.size()
				res.Flows[simFlow{From: s.to.i, To: s.from.i, Opener: s.from.i, Stream: s.stream}] += s.toInitiator.size()
			}
		}
		if s.at.After(done) {
			res.TailBytes[s.stream] += s.toHandler.size() + s.toInitiator.size()
		}
		if s.stream != rangesync.FetchStreamName {
			continue
		}
		to := s.from // the node that opened the fetch stream receives the chunks
		ms, err := protobuf.ReadMessages(bytes.NewReader(s.toInitiator.kept()), func() protobuf.Message { return new(pb.Delivery) })
		if err != nil {
			nb.tb.Fatalf("deliveries on a fetch stream: %v", err)
		}
		if delivered[to] == nil {
			delivered[to] = make(map[string]int)
		}
		for _, m := range ms {
			d := m.(*pb.Delivery)
			delivered[to][string(d.Address)+string(d.Stamp)]++
		}
	}
	for i, n := range nb.nodes {
		c, base := n.counters(), nb.baseline[i]
		nr := simNodeResult{
			Addr:         n.addr,
			Uploads:      n.uploads,
			Stored:       n.puts - n.uploads,
			Rounds:       len(n.rounds),
			RoundGap:     minGap(n.rounds),
			MetadataIn:   c.metaIn - base.metaIn,
			MetadataOut:  c.metaOut - base.metaOut,
			ChunksIn:     c.chunksIn - base.chunksIn,
			ChunksOut:    c.chunksOut - base.chunksOut,
			MoreRequests: c.mores - base.mores,
			WantClaims:   make(map[string]float64),
			PairwiseDiff: c.diffs - base.diffs,
		}
		for r, v := range c.claims {
			nr.WantClaims[r] = v - base.claims[r]
		}
		for k, v := range c.labeled {
			res.Counters[k] += v - base.labeled[k]
		}
		for _, m := range gathered(nb.tb, n.r.Metrics(), "_round_interval_seconds") {
			nr.RoundInterval = time.Duration(m.GetGauge().GetValue() * float64(time.Second))
		}
		for _, k := range delivered[n] {
			nr.Deliveries += k
			res.Duplicates += k - 1
		}
		res.Nodes = append(res.Nodes, nr)
	}

	for k, times := range nb.arrivals {
		interval, burst := rangesync.PeerSessionInterval, rangesync.PeerSessionBurst
		if k.status {
			interval, burst = rangesync.PeerStatusInterval, rangesync.PeerStatusBurst
		}
		res.RateExceeded += overRate(times, interval, burst)
	}
	return res
}

// minGap returns the shortest time between two consecutive times of ts (in
// order), 0 if there are fewer than two.
func minGap(ts []time.Time) time.Duration {
	var gap time.Duration
	for i := 1; i < len(ts); i++ {
		if d := ts[i].Sub(ts[i-1]); i == 1 || d < gap {
			gap = d
		}
	}
	return gap
}

// overRate returns how many of the arrivals at times (in order) a token
// bucket of burst tokens, regaining one per interval, refuses; a refused
// arrival takes no token.
func overRate(times []time.Time, interval time.Duration, burst int) int {
	tokens, refused := float64(burst), 0
	for i, at := range times {
		if i > 0 {
			tokens = math.Min(float64(burst), tokens+float64(at.Sub(times[i-1]))/float64(interval))
		}
		if tokens < 1 {
			refused++
			continue
		}
		tokens--
	}
	return refused
}

// randAddr returns a random address from the harness's generator.
func (nb *simNeighborhood) randAddr() swarm.Address {
	return swarm.NewAddress(nb.randBytes(swarm.HashSize))
}

func (nb *simNeighborhood) randBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(nb.rng.Uint32())
	}
	return b
}

// newStamp returns a stamp with random fields; the nodes accept every stamp.
func (nb *simNeighborhood) newStamp() *postage.Stamp {
	return postage.NewStamp(nb.randBytes(32), nb.randBytes(8), nb.randBytes(8), nb.randBytes(65))
}

// newChunk returns a valid content addressed chunk of ChunkSize random bytes
// in ChunkRoot with a random stamp.
func (nb *simNeighborhood) newChunk() swarm.Chunk {
	return nb.newCAC(nb.o.ChunkSize, nb.o.ChunkRoot).WithStamp(nb.newStamp())
}

// newCAC returns a valid content addressed chunk of size random bytes whose
// address lies in r, drawn until one does.
func (nb *simNeighborhood) newCAC(size int, r fingerprint.Range) swarm.Chunk {
	for {
		ch, err := cac.New(nb.randBytes(size))
		if err != nil {
			nb.tb.Fatal(err)
		}
		if r.Contains(ch.Address()) {
			return ch
		}
	}
}

// inRange returns a with its first r.Bits bits set to r's prefix.
func inRange(a swarm.Address, r fingerprint.Range) swarm.Address {
	b, p := a.Bytes(), r.Prefix.Bytes()
	for k := 0; k < int(r.Bits); k++ {
		mask := byte(0x80) >> (k % 8)
		b[k/8] = b[k/8]&^mask | p[k/8]&mask
	}
	return swarm.NewAddress(b)
}

// want returns the number of nodes whose root contains addr: the holders of
// a converged chunk.
func (nb *simNeighborhood) want(addr swarm.Address) int {
	n := 0
	for _, r := range nb.roots {
		if r.Contains(addr) {
			n++
		}
	}
	return n
}

// chunkKey returns the element ID of ch (address and stamp hash) as a map
// key.
func chunkKey(ch swarm.Chunk) (string, error) {
	sh, err := ch.Stamp().Hash()
	if err != nil {
		return "", err
	}
	return ch.Address().ByteString() + string(sh), nil
}

// putHook records a chunk the reserve stores and schedules the node's
// root-change signal, StatusMinInterval after the first change since the
// last signal (the Index's debounce, on the fake clock).
func (n *simNode) putHook(ch swarm.Chunk) error {
	nb := n.nb
	key, err := chunkKey(ch)
	if err != nil {
		return err
	}
	now := nb.clock.Now()
	nb.mu.Lock()
	n.puts++
	if _, ok := n.held[key]; !ok {
		n.held[key] = now
		if c := nb.chunks[key]; c != nil && n.rootRange.Contains(ch.Address()) {
			if c.holders++; c.holders == c.want {
				c.converged = now
				nb.converged++
			}
		}
	}
	signal := !n.debouncing
	n.debouncing = true
	nb.mu.Unlock()
	if signal {
		nb.clock.AfterFunc(rangesync.StatusMinInterval, n.rootChanged)
	}
	nb.touch()
	return nil
}

func (n *simNode) rootChanged() {
	n.nb.mu.Lock()
	n.debouncing = false
	n.nb.mu.Unlock()
	select {
	case n.root <- struct{}{}:
	default:
	}
}

// wrapProtocol returns svc's protocol with each handler counted in flight
// until it returns, given its end of the stream's pipes, and with a Headler
// that echoes the pairing header.
func (n *simNode) wrapProtocol() p2p.ProtocolSpec {
	spec := n.svc.Protocol()
	specs := make([]p2p.StreamSpec, len(spec.StreamSpecs))
	for i, ss := range spec.StreamSpecs {
		name, h := ss.Name, ss.Handler
		specs[i] = p2p.StreamSpec{
			Name:    name,
			Headler: func(hd p2p.Headers, _ swarm.Address) p2p.Headers { return hd },
			Handler: func(ctx context.Context, p p2p.Peer, s p2p.Stream) error {
				nb := n.nb
				nb.touch()
				defer func() { // NewStream counted the handler in flight
					nb.touch()
					nb.inflight.Add(-1)
				}()
				nb.arrived(n, p.Address, name)
				return h(ctx, p, nb.handlerEnd(s))
			},
		}
	}
	spec.StreamSpecs = specs
	return spec
}

// simService is the reconciler's view of a node's service: its calls are
// counted in flight and recorded (simSpan), and its root changes come from
// the harness.
type simService struct {
	*rangesync.Service
	n *simNode
}

var _ reconciler.Service = (*simService)(nil)

func (s *simService) SubscribeRootChange() (<-chan struct{}, func()) {
	return s.n.root, func() {}
}

// call counts a call of kind to peer in flight, which the reconciler made at
// entered, and returns its context, which collects the streams it opens; the
// returned function, given what a fetch stored and the call's error, ends it
// and records its span.
func (s *simService) call(ctx context.Context, kind string, peer swarm.Address, entered time.Time) (context.Context, func(stored int, err error)) {
	nb := s.n.nb
	end := nb.call()
	started := nb.clock.Now()
	c := new(simCall)
	return context.WithValue(ctx, simCallKey{}, c), func(stored int, err error) {
		now := nb.clock.Now()
		sp := simSpan{Kind: kind, Node: s.n.i, Peer: peer, At: entered.Sub(nb.start), Wait: started.Sub(entered), Duration: now.Sub(started), Stored: stored, Err: err}
		c.mu.Lock()
		sp.Stripe = c.stripe
		sp.Streams = len(c.streams)
		for _, p := range c.streams {
			if first := p.toInitiator.firstArrival(); p.stream == rangesync.FetchStreamName && !first.IsZero() {
				if d := first.Sub(started); sp.FirstDelivery == 0 || d < sp.FirstDelivery {
					sp.FirstDelivery = d
				}
			}
		}
		c.mu.Unlock()
		nb.mu.Lock()
		nb.spans = append(nb.spans, sp)
		nb.mu.Unlock()
		end()
	}
}

func (s *simService) SendStatusWith(ctx context.Context, peer swarm.Address, st rangesync.Status) (err error) {
	ctx, done := s.call(ctx, "status", peer, s.n.nb.clock.Now())
	defer func() { done(0, err) }()
	return s.Service.SendStatusWith(ctx, peer, st)
}

func (s *simService) ReconcileWith(ctx context.Context, peer swarm.Address, opts rangesync.ReconcileOptions) (wants []storer.ChunkID, err error) {
	nb := s.n.nb
	now := nb.clock.Now()
	ctx, done := s.call(ctx, "reconcile", peer, now)
	defer func() { done(0, err) }()
	nb.mu.Lock()
	nb.sessions = append(nb.sessions, now)
	nb.mu.Unlock()
	if opts.Striper == nil && len(opts.Open) > 0 {
		ctx.Value(simCallKey{}).(*simCall).stripe = true
	}
	wants, err = s.Service.ReconcileWith(ctx, peer, opts)
	if err == nil {
		nb.setWants(s.n, peer, true, len(wants))
	}
	return wants, err
}

func (s *simService) Fetch(ctx context.Context, peer swarm.Address, ids []storer.ChunkID) (n int, err error) {
	entered := s.n.nb.clock.Now()
	release, err := s.n.fetchSlot(ctx, peer)
	if err != nil {
		return 0, err
	}
	defer release()
	ctx, done := s.call(ctx, "fetch", peer, entered)
	defer func() { done(n, err) }()
	res, err := s.fetchOnClock(ctx, ids, func(ids []storer.ChunkID) (rangesync.FetchResult, error) {
		return s.Service.FetchWithResult(ctx, peer, ids)
	})
	return res.Stored, err
}

func (s *simService) FetchWithResult(ctx context.Context, peer swarm.Address, ids []storer.ChunkID) (res rangesync.FetchResult, err error) {
	entered := s.n.nb.clock.Now()
	release, err := s.n.fetchSlot(ctx, peer)
	if err != nil {
		return rangesync.FetchResult{}, err
	}
	defer release()
	ctx, done := s.call(ctx, "fetch", peer, entered)
	defer func() { done(res.Stored, err) }()
	return s.fetchOnClock(ctx, ids, func(ids []storer.ChunkID) (rangesync.FetchResult, error) {
		return s.Service.FetchWithResult(ctx, peer, ids)
	})
}

func (s *simService) FetchWithProgress(ctx context.Context, peer swarm.Address, ids []storer.ChunkID, stored func()) (res rangesync.FetchResult, err error) {
	entered := s.n.nb.clock.Now()
	release, err := s.n.fetchSlot(ctx, peer)
	if err != nil {
		return rangesync.FetchResult{}, err
	}
	defer release()
	ctx, done := s.call(ctx, "fetch", peer, entered)
	defer func() { done(res.Stored, err) }()
	return s.fetchOnClock(ctx, ids, func(ids []storer.ChunkID) (rangesync.FetchResult, error) {
		return s.Service.FetchWithProgress(ctx, peer, ids, stored)
	})
}

// fetchOnClock runs fetch over ids. With simOptions.FetchClockRate it runs
// it on one Want's worth of IDs at a time (MaxWantIDs), each after its share
// of the node-wide rate on the clock, and merges the results; it stops at the
// first error. The share is taken for every ID, also those the service then
// does not request (held, in the reserve), so it is an upper bound of the
// service's fetch limiter, which counts only the candidates of a Want.
func (s *simService) fetchOnClock(ctx context.Context, ids []storer.ChunkID, fetch func([]storer.ChunkID) (rangesync.FetchResult, error)) (rangesync.FetchResult, error) {
	rate := s.n.nb.o.FetchClockRate
	if rate <= 0 {
		return fetch(ids)
	}
	var res rangesync.FetchResult
	for len(ids) > 0 {
		batch := ids[:min(len(ids), rangesync.MaxWantIDs)]
		ids = ids[len(batch):]
		now := s.n.nb.clock.Now()
		s.n.fetchMu.Lock()
		start := s.n.fetchFree
		if start.Before(now) {
			start = now
		}
		s.n.fetchFree = start.Add(time.Duration(len(batch)) * time.Second / time.Duration(rate))
		wait := s.n.fetchFree.Sub(now)
		s.n.fetchMu.Unlock()
		if err := s.n.nb.sleep(ctx, wait); err != nil {
			return res, err
		}
		r, err := fetch(batch)
		res.Stored += r.Stored
		res.Undelivered = append(res.Undelivered, r.Undelivered...)
		res.OutsideRoot += r.OutsideRoot
		res.Held = append(res.Held, r.Held...)
		res.Deferred = append(res.Deferred, r.Deferred...)
		res.Unverified = append(res.Unverified, r.Unverified...)
		res.Rejected += r.Rejected
		if err != nil {
			return res, err
		}
	}
	return res, nil
}

// fetchSlot takes one of the node's fetch slots of peer, as many as the
// service has (rangesync.MaxFetchStreamsOut), and returns its release. It
// waits outside the calls in flight: a fetch past the service's slots would
// wait for one inside its call, and hold the clock while the fetch that
// holds the slot waits for it (a parked read, with simOptions.Delay).
func (n *simNode) fetchSlot(ctx context.Context, peer swarm.Address) (release func(), err error) {
	n.fetchMu.Lock()
	slots, ok := n.fetchSlots[peer.ByteString()]
	if !ok {
		slots = make(chan struct{}, rangesync.MaxFetchStreamsOut)
		n.fetchSlots[peer.ByteString()] = slots
	}
	n.fetchMu.Unlock()
	select {
	case slots <- struct{}{}:
		return func() { <-slots; n.nb.touch() }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// simTopology is a full mesh that records the time of every round of its
// node (a round follows the topology once).
type simTopology struct {
	n     *simNode
	peers []swarm.Address
}

func (t *simTopology) SubscribeTopologyChange() (<-chan struct{}, func()) {
	return make(chan struct{}), func() {}
}

func (t *simTopology) EachConnectedPeer(fn topology.EachPeerFunc, _ topology.Select) error {
	nb := t.n.nb
	now := nb.clock.Now()
	nb.mu.Lock()
	t.n.rounds = append(t.n.rounds, now)
	nb.mu.Unlock()
	for _, p := range t.peers {
		if stop, _, err := fn(p, 0); err != nil || stop {
			return err
		}
	}
	return nil
}

// simPairHeader is the header that pairs the two ends of a stream.
const simPairHeader = "sim-pair"

// simPair is one stream: its pipes in each direction.
type simPair struct {
	from, to               *simNode // the opener and the handler's node (nil if none)
	stream                 string
	at                     time.Time // opened
	toHandler, toInitiator *simPipe
}

// simStreamer opens the streams of a node: streamtest routes them, and their
// bytes go through a simPair.
type simStreamer struct {
	p2p.StreamerDisconnecter
	n *simNode
}

func (s *simStreamer) NewStream(ctx context.Context, addr swarm.Address, _ p2p.Headers, protocol, version, stream string) (p2p.Stream, error) {
	nb := s.n.nb
	if stream == rangesync.FetchStreamName && slices.ContainsFunc(nb.o.FetchOpenFails, func(i int) bool { return nb.nodes[i].addr.Equal(addr) }) {
		return nil, errSimOpenRefused
	}
	pair := &simPair{
		from:        s.n,
		stream:      stream,
		at:          nb.clock.Now(),
		toHandler:   newSimPipe(nb, false),
		toInitiator: newSimPipe(nb, stream == rangesync.FetchStreamName),
	}
	if i := slices.IndexFunc(nb.nodes, func(n *simNode) bool { return n.addr.Equal(addr) }); i >= 0 {
		pair.to = nb.nodes[i]
	}
	nb.mu.Lock()
	if nb.closed {
		nb.mu.Unlock()
		return nil, errSimClosed
	}
	nb.protocols[protocol] = true
	nb.streams = append(nb.streams, pair)
	nb.mu.Unlock()
	if c, ok := ctx.Value(simCallKey{}).(*simCall); ok {
		c.mu.Lock()
		c.streams = append(c.streams, pair)
		c.mu.Unlock()
	}
	id := strconv.FormatUint(nb.pairSeq.Add(1), 10)
	nb.pairs.Store(id, pair)
	nb.inflight.Add(1) // until the handler returns
	nb.touch()
	st, err := s.StreamerDisconnecter.NewStream(ctx, addr, p2p.Headers{simPairHeader: []byte(id)}, protocol, version, stream)
	if err != nil {
		nb.pairs.Delete(id)
		nb.inflight.Add(-1)
		return nil, err
	}
	end := &simEnd{Stream: st, in: pair.toInitiator, out: pair.toHandler}
	// the open's headers exchange is one round trip (the handler waits
	// for the first bytes meanwhile)
	if err := nb.sleep(ctx, 2*nb.o.Delay); err != nil {
		_ = end.Reset()
		return nil, err
	}
	return end, nil
}

// handlerEnd returns the handler's end of the stream s that a simStreamer
// opened.
func (nb *simNeighborhood) handlerEnd(s p2p.Stream) p2p.Stream {
	v, ok := nb.pairs.LoadAndDelete(string(s.ResponseHeaders()[simPairHeader]))
	if !ok {
		nb.tb.Error("stream without a pair")
		return s
	}
	pair := v.(*simPair)
	return &simEnd{Stream: s, in: pair.toHandler, out: pair.toInitiator}
}

// Errors of the harness's streams: errSimReset is what a read or a write of
// a stream that the other end reset returns (libp2p's network.ErrReset,
// which pkg/rangesync does not import); errSimExpectedEOF and
// errSimCloseTimeout are what FullClose returns when bytes arrive instead of
// the remote's end of the writes, and when that does not arrive within
// simCloseDeadline (libp2p's errExpectedEof and its read deadline);
// errSimClosed is what NewStream returns once the harness closed the streams
// (simNeighborhood.close), and errSimOpenRefused what it returns for a fetch
// stream to a node of simOptions.FetchOpenFails.
var (
	errSimReset        = errors.New("sim: stream reset")
	errSimExpectedEOF  = errors.New("sim: read on full close: expected eof")
	errSimCloseTimeout = errors.New("sim: full close deadline exceeded")
	errSimClosed       = errors.New("sim: neighborhood closed")
	errSimOpenRefused  = errors.New("sim: fetch stream open refused")
)

// simCloseDeadline bounds FullClose's wait for the remote's end of the
// writes, on the clock: libp2p's closeDeadline.
const simCloseDeadline = 30 * time.Second

// simEnd is one end of a stream. Close ends this end's writes: the remote
// reads the bytes written, then io.EOF. FullClose also waits for the remote's
// end of the writes, then ends this end's reads, and the remote's writes fail.
// Reset ends both directions: this end's reads and writes, and the remote's,
// fail with errSimReset, and the remote does not read the bytes it has not
// read yet. Close and Reset do not wait for the remote. With
// simOptions.Delay, the remote reads the bytes, the end of the writes and the
// reset Delay after they happened; its writes fail at once. The embedded
// streamtest stream gives the headers and the version, and is closed with
// it.
type simEnd struct {
	p2p.Stream
	in, out *simPipe
	mu      sync.Mutex
	closed  bool
}

func (e *simEnd) isClosed() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.closed
}

func (e *simEnd) Read(b []byte) (int, error) {
	if e.isClosed() {
		return 0, streamtest.ErrStreamClosed
	}
	return e.in.read(b)
}

func (e *simEnd) Write(b []byte) (int, error) {
	if e.isClosed() {
		return 0, streamtest.ErrStreamClosed
	}
	return e.out.write(b)
}

func (e *simEnd) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return streamtest.ErrStreamClosed
	}
	e.closed = true
	e.out.closeWrite(false)
	_ = e.Stream.Close()
	return nil
}

// FullClose ends this end's writes and then, as libp2p's FullClose
// (pkg/p2p/libp2p/stream.go), waits for the remote's end of the writes: it
// returns once that arrives, or resets the stream and returns an error when
// bytes arrive instead (or are unread), the remote resets the stream, or
// simCloseDeadline passes on the clock. The wait is parked.
func (e *simEnd) FullClose() error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return streamtest.ErrStreamClosed
	}
	e.closed = true
	e.mu.Unlock()
	e.out.closeWrite(false)
	if err := e.in.awaitEOF(simCloseDeadline); err != nil {
		e.out.closeWrite(true)
		e.in.closeRead(errSimReset, errSimReset)
		_ = e.Stream.Reset()
		return err
	}
	e.in.closeRead(io.EOF, streamtest.ErrStreamClosed)
	_ = e.Stream.FullClose()
	return nil
}

func (e *simEnd) Reset() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closed = true
	e.out.closeWrite(true)
	e.in.closeRead(errSimReset, errSimReset)
	_ = e.Stream.Reset()
	return nil
}

// simPipe is one direction of a stream: an unbounded buffer, so that a
// writer never waits for its reader. With a delay, what the writer does (its
// bytes, the end of its writes, a reset) reaches the reader that long later
// on the clock, in order.
//
// A reader waiting for the pipe is parked: it is not counted as work in
// flight (simNeighborhood.parked), since it waits either for its remote,
// which is in flight or parked itself, or for the clock to deliver what is
// on the way. An event that may let it go on (an arrival, a close of its
// end) counts it as working again before it wakes it. The streams of a call
// or handler are closed or reset before it returns (pkg/rangesync does so on
// every path), which also wakes a read the call left behind, so that no
// reader stays parked for a call that has ended.
type simPipe struct {
	nb      *simNeighborhood
	delay   time.Duration
	mu      sync.Mutex
	cond    sync.Cond
	buf     []byte       // arrived, unread
	first   time.Time    // when the first bytes arrived
	pending []simArrived // written, on the way (delay)
	due     time.Time    // the latest arrival time scheduled
	eof     bool         // arrived: the writer closed
	reset   bool         // arrived: the writer reset the stream
	rerr    error        // the reader's end closed: what its reads return
	werr    error        // the reader's end closed: what writes return
	parked  int          // readers counted in nb.parked
	written int          // bytes written
	keep    bool         // keep every byte written (all)
	all     []byte       // with keep
}

// simArrived is what a writer did, arriving at a time: bytes, the end of
// its writes or a reset.
type simArrived struct {
	at         time.Time
	data       []byte
	eof, reset bool
}

func newSimPipe(nb *simNeighborhood, keep bool) *simPipe {
	p := &simPipe{nb: nb, delay: nb.o.Delay, keep: keep}
	p.cond.L = &p.mu
	return p
}

func (p *simPipe) write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.werr != nil {
		return 0, p.werr
	}
	p.written += len(b)
	if p.keep {
		p.all = append(p.all, b...)
	}
	p.send(simArrived{data: slices.Clone(b)})
	return len(b), nil
}

// closeWrite ends the writes: the reader reads io.EOF after the bytes
// written, or errSimReset at once (reset) once it arrives.
func (p *simPipe) closeWrite(reset bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.send(simArrived{eof: !reset, reset: reset})
}

// send delivers a now, or schedules its arrival delay later. The caller
// holds p.mu.
func (p *simPipe) send(a simArrived) {
	if p.delay <= 0 {
		p.arriveLocked(a)
		return
	}
	a.at = p.nb.clock.Now().Add(p.delay)
	p.pending = append(p.pending, a)
	if a.at.After(p.due) {
		p.due = a.at
		p.nb.clock.AfterFunc(p.delay, p.deliver)
	}
}

// deliver delivers what has arrived by now.
func (p *simPipe) deliver() {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.nb.clock.Now()
	n := 0
	for ; n < len(p.pending) && !p.pending[n].at.After(now); n++ {
		p.arriveLocked(p.pending[n])
	}
	clear(p.pending[:n])
	p.pending = p.pending[n:]
}

func (p *simPipe) arriveLocked(a simArrived) {
	switch {
	case a.reset:
		p.reset, p.buf = true, nil
	case a.eof:
		p.eof = true
	default:
		if p.first.IsZero() {
			p.first = p.nb.clock.Now()
		}
		p.buf = append(p.buf, a.data...)
	}
	p.wakeLocked()
}

// closeRead ends the reader's end: its reads return rerr, the writer's
// writes werr.
func (p *simPipe) closeRead(rerr, werr error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.rerr == nil {
		p.rerr, p.werr = rerr, werr
	}
	p.wakeLocked()
}

// wakeLocked counts the parked readers as working again and wakes them.
func (p *simPipe) wakeLocked() {
	if p.parked > 0 {
		p.nb.parked.Add(-int64(p.parked))
		p.parked = 0
	}
	p.cond.Broadcast()
	p.nb.touch()
}

func (p *simPipe) read(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for {
		switch {
		case p.reset:
			return 0, errSimReset
		case p.rerr != nil && (len(p.buf) == 0 || !errors.Is(p.rerr, io.EOF)):
			return 0, p.rerr
		case len(p.buf) > 0:
			n := copy(b, p.buf)
			if p.buf = p.buf[n:]; len(p.buf) == 0 {
				p.buf = nil
			}
			return n, nil
		case p.eof:
			return 0, io.EOF
		}
		p.parked++
		p.nb.parked.Add(1)
		p.cond.Wait() // wakeLocked counted it as working again
	}
}

// awaitEOF waits, parked, until the writer's end of the writes arrives
// (FullClose). It returns errSimExpectedEOF if bytes arrive first or are
// unread, errSimReset on a reset, the reader's own close error, or
// errSimCloseTimeout once d passed on the clock.
func (p *simPipe) awaitEOF(d time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	var stop func()
	defer func() {
		if stop != nil {
			stop()
		}
	}()
	timedOut := false
	for {
		switch {
		case p.reset:
			return errSimReset
		case len(p.buf) > 0:
			return errSimExpectedEOF
		case p.eof:
			return nil
		case p.rerr != nil:
			if errors.Is(p.rerr, io.EOF) {
				return nil // the harness closed the stream
			}
			return p.rerr
		case timedOut:
			return errSimCloseTimeout
		}
		if stop == nil {
			stop = p.nb.clock.AfterFunc(d, func() {
				p.mu.Lock()
				defer p.mu.Unlock()
				timedOut = true
				p.wakeLocked()
			})
		}
		p.parked++
		p.nb.parked.Add(1)
		p.cond.Wait() // wakeLocked counted it as working again
	}
}

// kept returns every byte written to a pipe that keeps them.
func (p *simPipe) kept() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.all)
}

// firstArrival returns when the first bytes arrived, zero if none did.
func (p *simPipe) firstArrival() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.first
}

// size returns the number of bytes written to the pipe.
func (p *simPipe) size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.written
}

// simTimer is a pending timer of a simClock: a channel to send the time on,
// or a function to call. A stopped timer is dropped without firing.
type simTimer struct {
	at      time.Time
	seq     uint64
	c       chan time.Time
	f       func()
	stopped bool
}

type simTimers []*simTimer

func (h simTimers) Len() int { return len(h) }
func (h simTimers) Less(i, j int) bool {
	if !h[i].at.Equal(h[j].at) {
		return h[i].at.Before(h[j].at)
	}
	return h[i].seq < h[j].seq
}
func (h simTimers) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *simTimers) Push(x any)   { *h = append(*h, x.(*simTimer)) }
func (h *simTimers) Pop() any {
	old := *h
	t := old[len(old)-1]
	old[len(old)-1] = nil
	*h = old[:len(old)-1]
	return t
}

// simClock is the simulation's fake clock. Only the harness moves it
// (advance); timers fire in time order, and in the order they were set at one
// time.
type simClock struct {
	mu     sync.Mutex
	now    time.Time
	timers simTimers
	seq    uint64
	touch  func()
}

func (c *simClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// After returns a channel that receives the time once d has passed on the
// clock; at once for d <= 0.
func (c *simClock) After(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	c.schedule(d, ch, nil)
	return ch
}

// AfterFunc calls f on the harness goroutine once d has passed on the clock,
// unless stop was called before.
func (c *simClock) AfterFunc(d time.Duration, f func()) (stop func()) {
	tm := c.schedule(d, nil, f)
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		tm.stopped = true
	}
}

func (c *simClock) schedule(d time.Duration, ch chan time.Time, f func()) *simTimer {
	tm := &simTimer{c: ch, f: f}
	c.mu.Lock()
	if d <= 0 && ch != nil {
		ch <- c.now
	} else {
		c.seq++
		tm.at, tm.seq = c.now.Add(d), c.seq
		heap.Push(&c.timers, tm)
	}
	c.mu.Unlock()
	c.touch()
	return tm
}

// next returns the time of the earliest pending timer that was not stopped.
func (c *simClock) next() (time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for len(c.timers) > 0 && c.timers[0].stopped {
		heap.Pop(&c.timers)
	}
	if len(c.timers) == 0 {
		return time.Time{}, false
	}
	return c.timers[0].at, true
}

// advance moves the clock to t and fires, in order, every timer due by then.
func (c *simClock) advance(t time.Time) {
	for {
		c.mu.Lock()
		if len(c.timers) == 0 || c.timers[0].at.After(t) {
			if t.After(c.now) {
				c.now = t
			}
			c.mu.Unlock()
			return
		}
		tm := heap.Pop(&c.timers).(*simTimer)
		if tm.stopped {
			c.mu.Unlock()
			continue
		}
		if tm.at.After(c.now) {
			c.now = tm.at
		}
		now := c.now
		c.mu.Unlock()
		if tm.c != nil {
			tm.c <- now
		} else {
			tm.f()
		}
		c.touch()
	}
}

// diffsFrom returns the pairwise_diff samples observed from from on.
func (r *simResult) diffsFrom(from time.Duration) []float64 {
	var out []float64
	for _, s := range r.PairwiseDiff {
		if s.At >= from {
			out = append(out, s.Diff)
		}
	}
	return out
}

// exactDiffsFrom returns the pairwise_diff samples observed from from on by
// the side of a session whose difference is exact (SessionSample.Exact): the
// side that decoded the session's symbol streams, in the steady state the
// initiator, which decodes the responder's root stream. The other side
// derives its difference from counts of different times, which is
// approximate while the neighborhood changes (Session.Difference). A session
// with no difference is exact on both sides and counts its initiator's
// sample only, so each session counts at most once: a side that decoded a
// stream sends the peer a listing, which makes the peer's sample inexact.
func (r *simResult) exactDiffsFrom(from time.Duration) []float64 {
	var out []float64
	for _, s := range r.PairwiseDiff {
		if ss := s.Session; s.At >= from && ss.Exact && (ss.Initiator || ss.Difference > 0) {
			out = append(out, s.Diff)
		}
	}
	return out
}

// symbolsPerDifference returns the coded symbols the sessions observed from
// from on received, divided by the differences their decoders recovered
// (overshoot and streams that did not decode included), and the symbols.
func (r *simResult) symbolsPerDifference(from time.Duration) (perDiff float64, symbols uint64) {
	var recovered uint64
	for _, s := range r.PairwiseDiff {
		if s.At >= from {
			symbols += s.Session.SymbolsReceived
			recovered += s.Session.Recovered
		}
	}
	if recovered == 0 {
		return 0, symbols
	}
	return float64(symbols) / float64(recovered), symbols
}

// quantile returns the q-quantile of xs (nearest rank), 0 if empty.
func quantile(xs []float64, q float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := slices.Sorted(slices.Values(xs))
	return s[max(0, int(math.Ceil(q*float64(len(s))))-1)]
}
