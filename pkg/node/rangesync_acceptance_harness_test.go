// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build integration

package node_test

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"math/big"
	"math/rand/v2"
	"os"
	rtmetrics "runtime/metrics"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	dto "github.com/prometheus/client_model/go"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/node"
	"github.com/ethersphere/bee/v2/pkg/postage"
	batchstore "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	"github.com/ethersphere/bee/v2/pkg/puller"
	"github.com/ethersphere/bee/v2/pkg/pullsync"
	pullsyncpb "github.com/ethersphere/bee/v2/pkg/pullsync/pb"
	"github.com/ethersphere/bee/v2/pkg/rangesync"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/reconciler"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/statestore/leveldb"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// The acceptance suite of plan task T6.1 runs networks of in-process nodes
// that sync with pullsync, rangesync or both, as the flags --puller-enable
// and --rangesync-enable set them (proposal §8.3), and measures them.
//
// A node is built the way NewBee builds the parts the two flags change
// (pkg/node/node.go): ResolveSyncWiring decides the address index, the
// rangesync service and the puller; the storer is a real storer.DB (in
// memory) with its reserve; pullsync.New and, with rangesync,
// reconciler.New + rangesync.New(…, RangesyncOptions(r)) + SetService +
// Start; the protocols are added to the node's host; with the puller,
// puller.New on the node's state store; the combined syncer of the enabled
// drivers goes to StartReserveWorker, which starts it once the node's warm-up
// stabilization completes. NewBee itself cannot be built in a test (a full
// node always enables the chain backend; see the T6.2 entry "Node level
// without NewBee").
//
// Uploads arrive as pushsync stores them: the stamp is validated (the real
// postage.ValidStamp on a batch store holding the suite's batch) and the
// chunk is put into the reserves of the p nodes closest to its address.
//
// The harness instruments, without changing what the protocols see:
//   - the validStamp each protocol gets counts the deliveries it received,
//     per node and chunk ID (exact duplicates);
//   - the reserve putter the protocols get records when a chunk ID first
//     reached each node's reserve (convergence times);
//   - the transport counts the wire bytes of every protocol stream.

// accMode is the historical-sync configuration of a node: the flags
// --puller-enable (Puller) and --rangesync-enable (Rangesync).
type accMode struct {
	Puller, Rangesync bool
}

var (
	accPullsyncOnly  = accMode{Puller: true}
	accRangesyncOnly = accMode{Rangesync: true}
	accDual          = accMode{Puller: true, Rangesync: true}
)

func (m accMode) String() string {
	switch m {
	case accPullsyncOnly:
		return "P"
	case accRangesyncOnly:
		return "R"
	case accDual:
		return "D"
	}
	return "-"
}

// accOptions configure a network of the suite.
type accOptions struct {
	// Nodes is the number of nodes; Mode gives node i its flags.
	Nodes int
	Mode  func(i int) accMode
	// Radius is the storage radius of every node: the overlays share their
	// first Radius bits, and the chunks are generated in that neighborhood.
	// NodeRadius, if set, gives node i its own radius instead (its overlay
	// still shares the first Radius bits).
	Radius     uint8
	NodeRadius func(i int) uint8
	// Base chunks are held by every node at the start, as by a neighborhood
	// in sync (default 4500): with them no node starts far behind its peers,
	// and the root holds more than fingerprint.ScanLimit elements, so that
	// sessions read the root's symbol cache, as on a full reserve, instead of
	// scanning it. A node for which Empty reports true starts with an empty
	// reserve.
	Base  int
	Empty func(i int) bool
	// Uploads chunks (default 300) are pushed at Rate per second (default
	// 15), each to the Replication nodes closest to it (default 3).
	Uploads     int
	Rate        float64
	Replication int
	// Seed seeds the overlays and the chunk contents: two runs with the same
	// seed and sizes see the same overlays and upload the same chunks (their
	// stamps differ: the batch key and the timestamps are new each run).
	Seed uint64
	// Latency is the one-way latency of every stream (default 0).
	Latency time.Duration
	// Tail is how long the network must stay quiet (no session started)
	// after it converged before the measurement ends (default 10 s).
	Tail time.Duration
	// Limit bounds the time from the last upload to the end of the quiet
	// tail (default AC-C2's 120 s).
	Limit time.Duration
	// ReserveCapacity is every reserve's capacity (default 1<<16, no
	// eviction), ReserveWakeUpDuration the reserve worker's radius check
	// interval (default the storer's 30 min). WakeUp, if set, gives node i
	// its own interval instead (0 keeps ReserveWakeUpDuration).
	ReserveCapacity       int
	ReserveWakeUpDuration time.Duration
	WakeUp                func(i int) time.Duration
	// Hold, if set and true for node i, holds its reserve worker's radius
	// decrease until the harness releases it (accNode.release): the worker
	// reads a non-zero sync rate meanwhile. It stands for the moment the
	// reserve falls below its threshold, which the scenario picks.
	Hold func(i int) bool
	// Overlay, if set, returns node i's overlay from a random address and
	// the neighborhood's prefix; it must share the first Radius bits of the
	// prefix. By default the random address gets the first Radius bits.
	Overlay func(i int, random, prefix swarm.Address) swarm.Address
	// Window bounds the bytes a stream writer may send ahead of its reader,
	// per direction (0: unbounded; see accPipe).
	Window int
	// LargeDiff, if set, is every rangesync node's threshold of a striped
	// transfer: rangesync.Options.LargeDiff and reconciler.Options.LargeDiff
	// (the node leaves both at rangesync.LargeDiff). It scales a bootstrap
	// down to the suite's reserves.
	LargeDiff uint64
}

func (o accOptions) withDefaults() accOptions {
	if o.Base == 0 {
		o.Base = 4500
	}
	if o.Uploads == 0 {
		o.Uploads = 300
	}
	if o.Rate == 0 {
		o.Rate = 15
	}
	if o.Replication == 0 {
		o.Replication = 3
	}
	if o.Tail == 0 {
		o.Tail = 10 * time.Second
	}
	if o.Limit == 0 {
		o.Limit = 120 * time.Second
	}
	if o.ReserveCapacity == 0 {
		o.ReserveCapacity = 1 << 16
	}
	if o.ReserveWakeUpDuration == 0 {
		o.ReserveWakeUpDuration = 30 * time.Minute // the node's default
	}
	if o.Mode == nil {
		o.Mode = func(int) accMode { return accPullsyncOnly }
	}
	return o
}

// Protocols of a delivery (accDeliveries).
const (
	accViaPullsync = iota
	accViaRangesync
)

// accDeliveries counts the deliveries of one chunk ID to one node, by
// protocol, and records when the first of each protocol came.
type accDeliveries struct {
	n     [2]int
	first [2]time.Time
}

// accStabilizer is a node's warm-up stabilization (stabilization.Subscriber):
// the harness stabilizes the nodes once they are connected, as the detector
// of a node does after its peer connections settle.
type accStabilizer struct {
	c    chan struct{}
	once sync.Once
}

func newAccStabilizer() *accStabilizer { return &accStabilizer{c: make(chan struct{})} }

func (s *accStabilizer) Subscribe() (<-chan struct{}, func()) { return s.c, func() {} }

func (s *accStabilizer) IsStabilized() bool {
	select {
	case <-s.c:
		return true
	default:
		return false
	}
}

func (s *accStabilizer) stabilize() { s.once.Do(func() { close(s.c) }) }

// accNode is one node of the network.
type accNode struct {
	i       int
	overlay swarm.Address
	mode    accMode
	radius  uint8 // the radius it starts with

	host  *accHost
	topo  *accTopology
	stab  *accStabilizer
	db    *storer.DB
	store *accStore // db, as the protocols get it
	state io.Closer

	pullsync *pullsync.Syncer
	puller   *puller.Puller         // nil without --puller-enable
	rec      *reconciler.Reconciler // nil without --rangesync-enable
	svc      *rangesync.Service     // nil without --rangesync-enable
	rsCloser io.Closer              // the node's rangesync closer
	syncer   storer.Syncer          // the combined syncer
	gate     *accSyncGate           // syncer, as the reserve worker reads it
	cancel   context.CancelFunc     // ends the node's context, as Shutdown
	reg      *prometheus.Registry   // every metric of the node
	offline  bool                   // left the network (accHarness.leave)
}

// root returns the node's root: its overlay prefix of its storage radius.
func (n *accNode) root() fingerprint.Range {
	return fingerprint.NewRange(n.overlay, n.db.StorageRadius())
}

// release lets the node's reserve worker lower the radius (accOptions.Hold).
func (n *accNode) release() { n.gate.held.Store(false) }

// accSyncGate is the node's combined syncer as its reserve worker reads it:
// while held, SyncRate reports a non-zero rate, so the worker does not lower
// the radius (accOptions.Hold); otherwise it is the syncer's.
type accSyncGate struct {
	storer.Syncer
	held atomic.Bool
}

func (g *accSyncGate) SyncRate() float64 {
	if g.held.Load() {
		return 1
	}
	return g.Syncer.SyncRate()
}

// accStore is the reserve the node's protocols get: the node's storer.DB,
// whose putter records the arrivals, whose ReserveHasID records the IDs it
// found missing and whose ReserveIterateAddr counts the iterations over the
// whole root.
type accStore struct {
	*storer.DB
	h *accHarness
	i int

	rootIters atomic.Int64
}

// ReserveHasID records when the reserve last lacked the ID. Only rangesync
// calls it (pullsync calls ReserveHas): before a delivery, the last miss is
// the claim's re-check, or a later check by another session or fetch.
func (s *accStore) ReserveHasID(addr swarm.Address, stampHash []byte) (bool, error) {
	has, err := s.DB.ReserveHasID(addr, stampHash)
	if err == nil && !has {
		s.h.missed(s.i, fingerprint.NewID(addr, stampHash))
	}
	return has, err
}

// ReserveIterateAddr counts the iterations over the whole root (a scan or a
// listing of it): a session that reads the root's symbols from the root
// cache makes none.
func (s *accStore) ReserveIterateAddr(prefix swarm.Address, bits uint8, after []byte, cb func(storer.ChunkID) (bool, error)) error {
	if bits <= s.StorageRadius() {
		s.rootIters.Add(1)
	}
	return s.DB.ReserveIterateAddr(prefix, bits, after, cb)
}

func (s *accStore) ReservePutter() storage.Putter {
	p := s.DB.ReservePutter()
	return storage.PutterFunc(func(ctx context.Context, ch swarm.Chunk) error {
		if err := p.Put(ctx, ch); err != nil {
			return err
		}
		s.h.arrived(s.i, ch)
		return nil
	})
}

// accUpload is a chunk pushed during the run.
type accUpload struct {
	ch      swarm.Chunk
	id      fingerprint.ID
	at      time.Time
	holders []int // the nodes it was pushed to
	want    []int // the nodes whose root holds it
}

// accHarness is a network of the suite and what it measured.
type accHarness struct {
	tb     *testing.T
	o      accOptions
	logger log.Logger
	net    *accNet
	nodes  []*accNode

	rng        *rand.ChaCha8
	prefix     swarm.Address // the neighborhood
	base       []swarm.Chunk // the base chunks (validated)
	signer     crypto.Signer
	batch      *postage.Batch
	batchStore postage.Storer
	validStamp postage.ValidStampFn
	slots      map[uint32]uint32
	stampTime  uint64

	process *prometheus.Registry // bee_process_* of the test process

	mu         sync.Mutex
	arrivals   []map[fingerprint.ID]time.Time      // per node, first in the reserve
	deliveries []map[fingerprint.ID]*accDeliveries // per node
	placed     []map[fingerprint.ID]accPlacement   // per node, put by the harness
	pushed     []map[fingerprint.ID]time.Time      // per node, when the push's Put returned
	misses     []map[fingerprint.ID]time.Time      // per node, the last ReserveHasID that lacked the ID
	uploads    []*accUpload
	// fetchLat is, per first rangesync delivery of an ID to a node, the time
	// since the node's reserve last lacked the ID (the claim's re-check, or
	// a later check); pushSpan the longest push over its holders
	fetchLat []time.Duration
	pushSpan time.Duration

	psDeliveryBytes atomic.Uint64 // wire bytes of the pullsync Deliveries received

	sampler accSampler
}

const (
	accBucketDepth = 16
	accBatchDepth  = 24
)

// newAccHarness builds the nodes (not connected).
func newAccHarness(t *testing.T, o accOptions) *accHarness {
	t.Helper()

	o = o.withDefaults()
	var seed [32]byte
	binary.BigEndian.PutUint64(seed[:], o.Seed)
	h := &accHarness{
		tb:      t,
		o:       o,
		logger:  accLogger(),
		net:     newAccNet(o.Latency, o.Window),
		rng:     rand.NewChaCha8(seed),
		slots:   make(map[uint32]uint32),
		process: prometheus.NewRegistry(),
	}
	h.process.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{Namespace: "bee"}))

	key, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	h.signer = crypto.NewDefaultSigner(key)
	owner, err := h.signer.EthereumAddress()
	if err != nil {
		t.Fatal(err)
	}
	h.batch = &postage.Batch{
		ID:          h.randBytes(32),
		Value:       big.NewInt(1 << 40),
		Owner:       owner.Bytes(),
		Depth:       accBatchDepth,
		BucketDepth: accBucketDepth,
	}
	h.batchStore = batchstore.New(batchstore.WithBatch(h.batch))
	h.validStamp = postage.ValidStamp(h.batchStore)

	h.prefix = swarm.NewAddress(h.randBytes(swarm.HashSize))
	h.base = make([]swarm.Chunk, o.Base)
	for i := range h.base {
		valid, err := h.validStamp(h.newChunk())
		if err != nil {
			t.Fatal(err)
		}
		h.base[i] = valid
	}
	for i := range o.Nodes {
		h.addNode(h.nodeSpec(i))
	}
	return h
}

// accNodeSpec is what makes a node: its overlay, flags and radius, the base
// chunks it holds at the start, its reserve worker's interval and whether
// the harness holds its radius decrease.
type accNodeSpec struct {
	overlay swarm.Address
	mode    accMode
	radius  uint8
	base    []swarm.Chunk
	wakeUp  time.Duration
	hold    bool
}

// nodeSpec returns the spec of node i from the options: a new overlay, and
// the base chunks inside its root unless it starts empty.
func (h *accHarness) nodeSpec(i int) accNodeSpec {
	o := h.o
	s := accNodeSpec{mode: o.Mode(i), radius: o.Radius, wakeUp: o.ReserveWakeUpDuration}
	random := swarm.NewAddress(h.randBytes(swarm.HashSize))
	if o.Overlay != nil {
		s.overlay = o.Overlay(i, random, h.prefix)
	} else {
		s.overlay = withPrefix(random, h.prefix, o.Radius)
	}
	if o.NodeRadius != nil {
		s.radius = o.NodeRadius(i)
	}
	if o.WakeUp != nil {
		if d := o.WakeUp(i); d != 0 {
			s.wakeUp = d
		}
	}
	s.hold = o.Hold != nil && o.Hold(i)
	if o.Empty == nil || !o.Empty(i) {
		root := fingerprint.NewRange(s.overlay, s.radius)
		for _, ch := range h.base {
			if root.Contains(ch.Address()) {
				s.base = append(s.base, ch)
			}
		}
	}
	return s
}

// accLogger returns the nodes' logger: none, unless
// RANGESYNC_ACCEPTANCE_LOG sets a verbosity (for example "debug").
func accLogger() log.Logger {
	v := os.Getenv("RANGESYNC_ACCEPTANCE_LOG")
	if v == "" {
		return log.Noop
	}
	l, err := log.ParseVerbosityLevel(v)
	if err != nil {
		return log.Noop
	}
	return log.NewLogger("acceptance", log.WithSink(os.Stderr), log.WithVerbosity(l))
}

func (h *accHarness) randBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = h.rng.Read(b)
	return b
}

// withPrefix returns a with its first bits bits taken from prefix.
func withPrefix(a, prefix swarm.Address, bits uint8) swarm.Address {
	b := slices.Clone(a.Bytes())
	p := prefix.Bytes()
	for i := 0; i < int(bits); i++ {
		mask := byte(0x80) >> (i % 8)
		b[i/8] = b[i/8]&^mask | p[i/8]&mask
	}
	return swarm.NewAddress(b)
}

// newChunk returns a new stamped content-addressed chunk of swarm.ChunkSize
// random bytes in the neighborhood.
func (h *accHarness) newChunk() swarm.Chunk {
	data := make([]byte, swarm.ChunkSize)
	for {
		_, _ = h.rng.Read(data)
		ch, err := cac.New(data)
		if err != nil {
			h.tb.Fatal(err)
		}
		if swarm.Proximity(ch.Address().Bytes(), h.prefix.Bytes()) >= h.o.Radius {
			return ch.WithStamp(h.newStamp(ch.Address()))
		}
	}
}

// newStamp returns a valid stamp of the suite's batch for addr, with the
// next free index of its bucket and a newer timestamp than every earlier one.
func (h *accHarness) newStamp(addr swarm.Address) *postage.Stamp {
	bucket := binary.BigEndian.Uint32(addr.Bytes()[:4]) >> (32 - accBucketDepth)
	slot := h.slots[bucket]
	h.slots[bucket]++
	if slot >= 1<<(accBatchDepth-accBucketDepth) {
		h.tb.Fatal("bucket full")
	}
	index := make([]byte, 8)
	binary.BigEndian.PutUint32(index, bucket)
	binary.BigEndian.PutUint32(index[4:], slot)
	h.stampTime++
	ts := make([]byte, 8)
	binary.BigEndian.PutUint64(ts, uint64(time.Now().UnixNano())+h.stampTime)
	digest, err := postage.ToSignDigest(addr.Bytes(), h.batch.ID, index, ts)
	if err != nil {
		h.tb.Fatal(err)
	}
	sig, err := h.signer.Sign(digest)
	if err != nil {
		h.tb.Fatal(err)
	}
	return postage.NewStamp(h.batch.ID, index, ts, sig)
}

// restamp returns ch with a newer stamp of the same batch and index: a later
// timestamp and a new signature (§7.7: the same chunk stamped twice).
func (h *accHarness) restamp(ch swarm.Chunk) swarm.Chunk {
	st := ch.Stamp()
	h.stampTime++
	ts := make([]byte, 8)
	binary.BigEndian.PutUint64(ts, max(binary.BigEndian.Uint64(st.Timestamp())+1, uint64(time.Now().UnixNano())+h.stampTime))
	digest, err := postage.ToSignDigest(ch.Address().Bytes(), st.BatchID(), st.Index(), ts)
	if err != nil {
		h.tb.Fatal(err)
	}
	sig, err := h.signer.Sign(digest)
	if err != nil {
		h.tb.Fatal(err)
	}
	return swarm.NewChunk(ch.Address(), ch.Data()).WithStamp(postage.NewStamp(st.BatchID(), st.Index(), ts, sig))
}

func chunkIDOf(ch swarm.Chunk) (fingerprint.ID, error) {
	sh, err := ch.Stamp().Hash()
	if err != nil {
		return fingerprint.ID{}, err
	}
	return fingerprint.NewID(ch.Address(), sh), nil
}

// addNode builds the next node of spec, as NewBee builds the parts the sync
// flags change, with spec's base chunks in its reserve.
func (h *accHarness) addNode(spec accNodeSpec) *accNode {
	tb := h.tb
	tb.Helper()

	i, overlay, mode, radius := len(h.nodes), spec.overlay, spec.mode, spec.radius
	addrIndex, registerRangesync, buildPuller, err := node.ResolveSyncWiring(true, false, mode.Rangesync, mode.Puller)
	if err != nil {
		tb.Fatal(err)
	}
	n := &accNode{
		i:       i,
		overlay: overlay,
		mode:    mode,
		radius:  radius,
		topo:    newAccTopology(overlay),
		stab:    newAccStabilizer(),
		reg:     prometheus.NewRegistry(),
	}
	h.mu.Lock()
	h.arrivals = append(h.arrivals, make(map[fingerprint.ID]time.Time))
	h.deliveries = append(h.deliveries, make(map[fingerprint.ID]*accDeliveries))
	h.placed = append(h.placed, make(map[fingerprint.ID]accPlacement))
	h.pushed = append(h.pushed, make(map[fingerprint.ID]time.Time))
	h.misses = append(h.misses, make(map[fingerprint.ID]time.Time))
	h.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	n.cancel = cancel
	n.db, err = storer.New(ctx, "", &storer.Options{
		Address:               overlay,
		Logger:                h.logger,
		Batchstore:            h.batchStore,
		ValidStamp:            h.validStamp,
		RadiusSetter:          n.topo,
		StartupStabilizer:     n.stab,
		ReserveCapacity:       h.o.ReserveCapacity,
		ReserveWakeUpDuration: spec.wakeUp,
		ReserveAddrIndex:      addrIndex,
		CacheCapacity:         1000,
	})
	if err != nil {
		cancel()
		tb.Fatal(err)
	}
	n.store = &accStore{DB: n.db, h: h, i: i}
	for _, ch := range spec.base {
		h.put(n, ch, accBase)
	}

	n.host = h.net.newHost(overlay, n.topo)
	unwrap := func(swarm.Chunk) {}
	gsoc := func(*soc.SOC) {}
	n.pullsync = pullsync.New(n.host, n.store, unwrap, gsoc, h.counting(i, accViaPullsync), h.logger, pullsync.DefaultMaxPage)
	if registerRangesync {
		// LargeDiff is the one deviation from the node's wiring, and only
		// where a scenario sets it
		n.rec = reconciler.New(overlay, n.topo, nil, h.logger, reconciler.Options{LargeDiff: h.o.LargeDiff})
		opts := node.RangesyncOptions(n.rec)
		opts.LargeDiff = h.o.LargeDiff
		n.svc = rangesync.New(n.host, n.store, overlay, unwrap, gsoc, h.counting(i, accViaRangesync), h.logger, opts)
		n.rsCloser = node.NewRangesyncCloser(n.rec, n.svc)
		n.rec.SetService(n.svc)
		if err := n.svc.Start(); err != nil {
			tb.Fatalf("node %d: rangesync start: %v", i, err)
		}
	}
	n.host.AddProtocol(n.pullsync.Protocol())
	if registerRangesync {
		n.host.AddProtocol(n.svc.Protocol())
	}

	state, err := leveldb.NewInMemoryStateStore(h.logger)
	if err != nil {
		tb.Fatal(err)
	}
	n.state = state
	var syncers []storer.Syncer
	if buildPuller {
		n.puller = puller.New(overlay, state, n.topo, n.db, n.pullsync, n.host, h.logger, puller.Options{})
		syncers = append(syncers, n.puller)
	}
	if n.rec != nil {
		syncers = append(syncers, n.rec)
	}
	n.syncer = node.NewCombinedSyncer(syncers...)
	n.gate = &accSyncGate{Syncer: n.syncer}
	n.gate.held.Store(spec.hold)
	n.db.StartReserveWorker(ctx, n.gate, func() (uint8, error) { return radius, nil }, nil)

	register := func(reg *prometheus.Registry, cs ...prometheus.Collector) {
		for _, c := range cs {
			if err := reg.Register(c); err != nil {
				var are prometheus.AlreadyRegisteredError
				if !errors.As(err, &are) {
					tb.Fatalf("node %d: register metrics: %v", i, err)
				}
			}
		}
	}
	register(n.reg, n.pullsync.Metrics()...)
	register(n.reg, n.db.Metrics()...)
	if n.puller != nil {
		register(n.reg, n.puller.Metrics()...)
	}
	if n.svc != nil {
		register(n.reg, n.svc.Metrics()...)
		register(n.reg, n.rec.Metrics()...)
	}
	h.nodes = append(h.nodes, n)
	return n
}

// counting returns the validStamp of node i's protocol via: it counts the
// delivery, then validates as the node does.
func (h *accHarness) counting(i, via int) postage.ValidStampFn {
	return func(ch swarm.Chunk) (swarm.Chunk, error) {
		h.delivered(i, via, ch)
		return h.validStamp(ch)
	}
}

func (h *accHarness) delivered(i, via int, ch swarm.Chunk) {
	id, err := chunkIDOf(ch)
	if err != nil {
		return
	}
	if via == accViaPullsync {
		stamp, err := ch.Stamp().MarshalBinary()
		if err == nil {
			d := pullsyncpb.Delivery{Address: ch.Address().Bytes(), Data: ch.Data(), Stamp: stamp}
			n := d.Size()
			h.psDeliveryBytes.Add(uint64(n + uvarintSize(uint64(n))))
		}
	}
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	d, ok := h.deliveries[i][id]
	if !ok {
		d = new(accDeliveries)
		h.deliveries[i][id] = d
	}
	if d.n[via]++; d.n[via] == 1 {
		d.first[via] = now
		if at, ok := h.misses[i][id]; ok && via == accViaRangesync {
			h.fetchLat = append(h.fetchLat, now.Sub(at))
		}
	}
}

// missed records that node i's reserve lacked id.
func (h *accHarness) missed(i int, id fingerprint.ID) {
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.misses[i][id] = now
}

func uvarintSize(x uint64) int {
	n := 1
	for ; x >= 0x80; x >>= 7 {
		n++
	}
	return n
}

// arrived records the first arrival of ch in node i's reserve.
func (h *accHarness) arrived(i int, ch swarm.Chunk) {
	id, err := chunkIDOf(ch)
	if err != nil {
		return
	}
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.arrivals[i][id]; !ok {
		h.arrivals[i][id] = now
	}
}

// accPlacement is how the harness put a chunk into a reserve.
type accPlacement int

const (
	accBase accPlacement = iota + 1 // before the start
	accPush                         // pushed during the run
)

// put stores ch in node n's reserve. A pushed chunk is stored as pushsync
// stores it: its stamp is validated, then the chunk is put. A base chunk was
// validated once when it was made (newAccHarness): it stands for a reserve
// synced before the start.
func (h *accHarness) put(n *accNode, ch swarm.Chunk, how accPlacement) {
	id, err := chunkIDOf(ch)
	if err != nil {
		h.tb.Fatal(err)
	}
	h.mu.Lock()
	h.placed[n.i][id] = how
	h.mu.Unlock()
	if how == accPush {
		if ch, err = h.validStamp(ch); err != nil {
			h.tb.Fatalf("node %d: stamp: %v", n.i, err)
		}
	}
	if err := n.store.ReservePutter().Put(context.Background(), ch); err != nil {
		h.tb.Fatalf("node %d: reserve put: %v", n.i, err)
	}
	if how == accPush {
		now := time.Now()
		h.mu.Lock()
		h.pushed[n.i][id] = now
		h.mu.Unlock()
	}
}

// push puts ch into the reserves of the Replication online nodes closest to
// it (pushsync does not reach a node that left).
func (h *accHarness) push(ch swarm.Chunk) {
	order := make([]int, 0, len(h.nodes))
	for _, n := range h.nodes {
		if !n.offline {
			order = append(order, n.i)
		}
	}
	sort.Slice(order, func(a, b int) bool {
		c, _ := swarm.DistanceCmp(ch.Address(), h.nodes[order[a]].overlay, h.nodes[order[b]].overlay)
		return c > 0
	})
	id, err := chunkIDOf(ch)
	if err != nil {
		h.tb.Fatal(err)
	}
	u := &accUpload{ch: ch, id: id, at: time.Now(), holders: order[:min(h.o.Replication, len(order))]}
	for _, n := range h.nodes {
		if h.expected(n, ch.Address()) {
			u.want = append(u.want, n.i)
		}
	}
	h.mu.Lock()
	h.uploads = append(h.uploads, u)
	h.mu.Unlock()
	for _, i := range u.holders {
		h.put(h.nodes[i], ch, accPush)
	}
	span := time.Since(u.at)
	h.mu.Lock()
	h.pushSpan = max(h.pushSpan, span)
	h.mu.Unlock()
}

// start connects every pair of nodes, stabilizes them (the reserve workers
// set the radius and start the syncers) and waits until the network is
// ready (waitReady).
func (h *accHarness) start() {
	h.tb.Helper()

	for a := range h.nodes {
		for b := a + 1; b < len(h.nodes); b++ {
			if err := h.net.connect(h.nodes[a].host, h.nodes[b].host); err != nil {
				h.tb.Fatal(err)
			}
		}
	}
	for _, n := range h.nodes {
		n.stab.stabilize()
	}
	h.sampler.start(h)
	h.waitReady()
}

// waitReady waits until every node runs at its radius and its sync drivers
// are idle on the base set: every rangesync node's index is at its root, its
// peers hold its current Status and its start-up guard lifted (SyncRate 0);
// every puller has finished its historical sync (only live workers left).
func (h *accHarness) waitReady() {
	h.tb.Helper()

	accWait(h.tb, "the network to be ready", 2*time.Minute, func() (bool, string) {
		for _, n := range h.nodes {
			if n.db.StorageRadius() != n.radius {
				return false, fmt.Sprintf("node %d radius %d", n.i, n.db.StorageRadius())
			}
			if n.svc != nil {
				st, err := n.svc.LocalStatus()
				if err != nil || !st.Root.Equal(n.root()) {
					return false, fmt.Sprintf("node %d index", n.i)
				}
				for _, p := range h.nodes {
					if p == n || p.svc == nil {
						continue
					}
					if got, ok := p.svc.LastStatus(n.overlay); !ok || got.Summary != st.Summary || !got.Root.Equal(st.Root) {
						return false, fmt.Sprintf("node %d status at node %d", n.i, p.i)
					}
				}
				if r := n.rec.SyncRate(); r != 0 {
					return false, fmt.Sprintf("node %d sync rate %v", n.i, r)
				}
			}
			if n.puller != nil {
				want := float64(pullerLiveWorkers(n, n.radius))
				if got := gatherAcc(h.tb, n.reg).value("bee_puller_worker"); got != want {
					return false, fmt.Sprintf("node %d puller workers %v, want %v", n.i, got, want)
				}
			}
		}
		return true, ""
	})
}

// pullerLiveWorkers returns the sync workers node n's puller runs at radius
// once its historical sync is done: a live worker per bin from the radius up
// for each connected peer in the neighborhood, and one for the peer's bin
// for a peer at most 2 bins below the radius (puller.go, syncPeer).
func pullerLiveWorkers(n *accNode, radius uint8) int {
	peers, pos := n.topo.byBin()
	workers := 0
	for i := range peers {
		switch {
		case pos[i] >= radius:
			workers += int(swarm.MaxBins) - int(radius)
		case radius-pos[i] <= 2:
			workers++
		}
	}
	return workers
}

// accWait polls cond until it holds, failing tb after limit with what cond
// last reported.
func accWait(tb testing.TB, what string, limit time.Duration, cond func() (bool, string)) {
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
		time.Sleep(50 * time.Millisecond)
	}
}

// expected reports whether node n's root holds addr.
func (h *accHarness) expected(n *accNode, addr swarm.Address) bool {
	return n.root().Contains(addr)
}

// join adds a node of spec to the running network: it connects to every
// online node, then stabilizes (its reserve worker sets its radius and starts
// its sync drivers), as a node that starts next to a running neighborhood.
func (h *accHarness) join(spec accNodeSpec) *accNode {
	h.tb.Helper()

	n := h.addNode(spec)
	h.sampler.add(h, n)
	for _, p := range h.nodes {
		if p == n || p.offline {
			continue
		}
		if err := h.net.connect(n.host, p.host); err != nil {
			h.tb.Fatal(err)
		}
	}
	n.stab.stabilize()
	return n
}

// leave disconnects node n from every peer: it is offline until rejoin.
func (h *accHarness) leave(n *accNode) {
	h.tb.Helper()

	for _, p := range h.nodes {
		if p != n && h.net.conn(n.overlay, p.overlay) != nil {
			if err := h.net.disconnect(n.host, p.host); err != nil {
				h.tb.Fatal(err)
			}
		}
	}
	n.offline = true
}

// rejoin connects node n to every online node again.
func (h *accHarness) rejoin(n *accNode) {
	h.tb.Helper()

	for _, p := range h.nodes {
		if p == n || p.offline {
			continue
		}
		if err := h.net.connect(n.host, p.host); err != nil {
			h.tb.Fatal(err)
		}
	}
	n.offline = false
}

// pending returns the number of (upload, node) pairs where the node's root
// holds the upload and its reserve does not yet, and the time the last
// arrival completed an upload.
func (h *accHarness) pending() (missing int, last time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, u := range h.uploads {
		for _, i := range u.want {
			at, ok := h.arrivals[i][u.id]
			if !ok {
				missing++
				continue
			}
			if at.After(last) {
				last = at
			}
		}
	}
	return missing, last
}

// close shuts the nodes down in NewBee's Shutdown order: the sync services
// at once and the node contexts, then the transport, then the storers and
// the state stores.
func (h *accHarness) close() {
	h.tb.Helper()

	h.sampler.stop()
	var wg sync.WaitGroup
	errs := make(chan error, 3*len(h.nodes))
	for _, n := range h.nodes {
		closers := []io.Closer{n.pullsync}
		if n.puller != nil {
			closers = append(closers, n.puller)
		}
		if n.rsCloser != nil {
			closers = append(closers, n.rsCloser)
		}
		for _, c := range closers {
			wg.Add(1)
			go func(c io.Closer) {
				defer wg.Done()
				if err := c.Close(); err != nil {
					errs <- err
				}
			}(c)
		}
		n.cancel()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		h.tb.Error(err)
	}
	if err := h.net.close(time.Minute); err != nil {
		h.tb.Error(err)
	}
	for _, n := range h.nodes {
		if err := n.db.Close(); err != nil {
			h.tb.Error(err)
		}
		if err := n.state.Close(); err != nil {
			h.tb.Error(err)
		}
	}
}

// accSampler samples, every accSampleInterval, every rangesync node's index
// memory and round interval, the sum of the sessions the reconcilers
// started, and the live Go heap of the process (as of its last GC).
type accSampler struct {
	quit chan struct{}
	done chan struct{}

	mu           sync.Mutex
	sampled      []*accSampled // per node, nil for a node without rangesync
	indexPeak    []float64     // per node, since resetPeaks
	indexPeakRun []float64     // per node, since start
	sessions     float64
	sessionsAt   time.Time // the last change of sessions
	heapPeak     uint64
	// the smallest and the largest round interval of the rangesync nodes
	// since resetPeaks, seconds
	roundMin, roundMax float64
	// rateMax is the largest sync rate of a reconciler since resetPeaks
	// (AC-N1: 0 in steady state with live uploads)
	rateMax float64
}

const accSampleInterval = 100 * time.Millisecond

func (s *accSampler) start(h *accHarness) {
	s.quit, s.done = make(chan struct{}), make(chan struct{})
	s.roundMin = math.Inf(1)
	for _, n := range h.nodes {
		s.add(h, n)
	}
	go func() {
		defer close(s.done)
		heap := []rtmetrics.Sample{{Name: "/gc/heap/live:bytes"}}
		t := time.NewTicker(accSampleInterval)
		defer t.Stop()
		for {
			var sessions, rate float64
			roundMin, roundMax := math.Inf(1), 0.0
			s.mu.Lock()
			sampled := slices.Clone(s.sampled)
			s.mu.Unlock()
			peaks := make([]float64, len(sampled))
			for i, c := range sampled {
				if c == nil {
					continue
				}
				peaks[i] = collectSum(c.index)
				sessions += collectSum(c.sessions)
				round := collectSum(c.round)
				roundMin, roundMax = min(roundMin, round), max(roundMax, round)
				rate = max(rate, collectSum(c.rate))
			}
			rtmetrics.Read(heap)
			now := time.Now()
			s.mu.Lock()
			for i, p := range peaks {
				s.indexPeak[i] = max(s.indexPeak[i], p)
				s.indexPeakRun[i] = max(s.indexPeakRun[i], p)
			}
			if sessions != s.sessions {
				s.sessions, s.sessionsAt = sessions, now
			}
			if heap[0].Value.Kind() == rtmetrics.KindUint64 {
				s.heapPeak = max(s.heapPeak, heap[0].Value.Uint64())
			}
			if roundMax > 0 {
				s.roundMin, s.roundMax = min(s.roundMin, roundMin), max(s.roundMax, roundMax)
			}
			s.rateMax = max(s.rateMax, rate)
			s.mu.Unlock()
			select {
			case <-s.quit:
				return
			case <-t.C:
			}
		}
	}()
}

// add samples node n from now on (a node that joins after start).
func (s *accSampler) add(h *accHarness, n *accNode) {
	var c *accSampled
	if n.svc != nil {
		c = &accSampled{
			index:    findCollector(h.tb, "bee_rangesync_index_memory_bytes", n.svc.Metrics()...),
			sessions: findCollector(h.tb, "bee_rangesync_sessions_total", n.rec.Metrics()...),
			round:    findCollector(h.tb, "bee_rangesync_round_interval_seconds", n.rec.Metrics()...),
			rate:     findCollector(h.tb, "bee_rangesync_sync_rate", n.rec.Metrics()...),
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.sampled) <= n.i {
		s.sampled = append(s.sampled, nil)
		s.indexPeak = append(s.indexPeak, 0)
		s.indexPeakRun = append(s.indexPeakRun, 0)
	}
	s.sampled[n.i] = c
}

// accSampled is the collectors a sampler reads of a rangesync node.
type accSampled struct {
	index, sessions, round, rate prometheus.Collector
}

// findCollector returns the collector of cs that describes the metric name.
func findCollector(tb testing.TB, name string, cs ...prometheus.Collector) prometheus.Collector {
	tb.Helper()

	for _, c := range cs {
		ch := make(chan *prometheus.Desc, 64) // every collector of a node describes fewer
		c.Describe(ch)
		close(ch)
		found := false
		for d := range ch {
			found = found || strings.Contains(d.String(), `fqName: "`+name+`"`)
		}
		if found {
			return c
		}
	}
	tb.Fatalf("no collector of %s", name)
	return nil
}

// collectSum returns the sum of the values of the counters and gauges c
// collects: cheaper than a registry's Gather, so that sampling stays out of
// the CPU the scenarios compare.
func collectSum(c prometheus.Collector) float64 {
	ch := make(chan prometheus.Metric, 64) // the sampled collectors collect fewer
	c.Collect(ch)
	close(ch)
	var sum float64
	for m := range ch {
		var pb dto.Metric
		if err := m.Write(&pb); err != nil {
			continue
		}
		sum += pb.GetCounter().GetValue() + pb.GetGauge().GetValue() + pb.GetUntyped().GetValue()
	}
	return sum
}

func (s *accSampler) stop() {
	if s.quit == nil {
		return
	}
	close(s.quit)
	<-s.done
	s.quit = nil
}

// resetPeaks restarts the peaks (index memory, heap) from now.
func (s *accSampler) resetPeaks() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.indexPeak)
	s.heapPeak = 0
	s.roundMin, s.roundMax = math.Inf(1), 0
	s.rateMax = 0
}

// lastSession returns the sessions started so far and when their number
// last changed.
func (s *accSampler) lastSession() (float64, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions, s.sessionsAt
}

// accMetrics is a gathered set of metrics: counter and gauge values and
// histograms, by name and labels (name{k="v",…}).
type accMetrics struct {
	values  map[string]float64
	counter map[string]bool
	hists   map[string]accHist
}

// accHist is a histogram: cumulative counts per upper bound.
type accHist struct {
	bounds []float64
	cum    []uint64
	count  uint64
	sum    float64
}

func gatherAcc(tb testing.TB, reg prometheus.Gatherer) accMetrics {
	tb.Helper()

	mfs, err := reg.Gather()
	if err != nil {
		tb.Errorf("gather metrics: %v", err)
	}
	m := accMetrics{values: make(map[string]float64), counter: make(map[string]bool), hists: make(map[string]accHist)}
	for _, mf := range mfs {
		for _, mt := range mf.GetMetric() {
			key := mf.GetName() + labelString(mt.GetLabel())
			switch mf.GetType() {
			case dto.MetricType_COUNTER:
				m.values[key] = mt.GetCounter().GetValue()
				m.counter[key] = true
			case dto.MetricType_GAUGE:
				m.values[key] = mt.GetGauge().GetValue()
			case dto.MetricType_UNTYPED:
				m.values[key] = mt.GetUntyped().GetValue()
			case dto.MetricType_HISTOGRAM:
				hg := mt.GetHistogram()
				hs := accHist{count: hg.GetSampleCount(), sum: hg.GetSampleSum()}
				for _, b := range hg.GetBucket() {
					hs.bounds = append(hs.bounds, b.GetUpperBound())
					hs.cum = append(hs.cum, b.GetCumulativeCount())
				}
				m.hists[key] = hs
			}
		}
	}
	return m
}

func labelString(ls []*dto.LabelPair) string {
	if len(ls) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ls))
	for _, l := range ls {
		parts = append(parts, fmt.Sprintf("%s=%q", l.GetName(), l.GetValue()))
	}
	sort.Strings(parts)
	return "{" + strings.Join(parts, ",") + "}"
}

// value returns the value of the metric key (name{labels}), 0 if absent.
func (m accMetrics) value(key string) float64 { return m.values[key] }

// byLabel returns the values of the metric name by its single label's value.
func (m accMetrics) byLabel(name string) map[string]float64 {
	out := make(map[string]float64)
	for k, v := range m.values {
		if !strings.HasPrefix(k, name+"{") {
			continue
		}
		l := k[len(name)+1 : len(k)-1]
		if i := strings.IndexByte(l, '"'); i >= 0 {
			l = strings.Trim(l[i:], `"`)
		}
		out[l] += v
	}
	return out
}

// since returns m minus prev for counters and histograms; gauges keep m's
// value.
func (m accMetrics) since(prev accMetrics) accMetrics {
	out := accMetrics{values: make(map[string]float64), counter: m.counter, hists: make(map[string]accHist)}
	for k, v := range m.values {
		if m.counter[k] {
			v -= prev.values[k]
		}
		out.values[k] = v
	}
	for k, hs := range m.hists {
		p, ok := prev.hists[k]
		d := accHist{bounds: hs.bounds, cum: slices.Clone(hs.cum), count: hs.count, sum: hs.sum}
		if ok && len(p.cum) == len(hs.cum) {
			for i := range d.cum {
				d.cum[i] -= p.cum[i]
			}
			d.count -= p.count
			d.sum -= p.sum
		}
		out.hists[k] = d
	}
	return out
}

// merge returns the sum of histograms with the same bounds.
func mergeHists(hs ...accHist) accHist {
	var out accHist
	for _, h := range hs {
		if len(h.bounds) == 0 {
			continue
		}
		if out.bounds == nil {
			out.bounds = h.bounds
			out.cum = make([]uint64, len(h.cum))
		}
		for i := range h.cum {
			out.cum[i] += h.cum[i]
		}
		out.count += h.count
		out.sum += h.sum
	}
	return out
}

// quantile returns the upper bound of the bucket that holds the q quantile,
// +Inf past the last bucket and NaN without observations.
func (hs accHist) quantile(q float64) float64 {
	if hs.count == 0 {
		return math.NaN()
	}
	rank := uint64(math.Ceil(q * float64(hs.count)))
	for i, c := range hs.cum {
		if c >= rank {
			return hs.bounds[i]
		}
	}
	return math.Inf(1)
}

// accSnapshot is the state of the counters at an instant.
type accSnapshot struct {
	at      time.Time
	nodes   []accMetrics
	process accMetrics
	wire    []map[string]uint64 // per node, bytes written by "protocol/stream"
	wireIn  []map[string]uint64 // per node, bytes written to it by "protocol/stream"
	psBytes uint64              // pullsync Delivery bytes received
}

// node returns node i's metrics, none for a node that joined after the
// snapshot.
func (s accSnapshot) node(i int) accMetrics {
	if i < len(s.nodes) {
		return s.nodes[i]
	}
	return accMetrics{values: map[string]float64{}, counter: map[string]bool{}, hists: map[string]accHist{}}
}

// wireOf returns node i's wire bytes written, none for a node that joined
// after the snapshot.
func (s accSnapshot) wireOf(i int) map[string]uint64 {
	if i < len(s.wire) {
		return s.wire[i]
	}
	return nil
}

// wireInOf returns the wire bytes written to node i, none for a node that
// joined after the snapshot.
func (s accSnapshot) wireInOf(i int) map[string]uint64 {
	if i < len(s.wireIn) {
		return s.wireIn[i]
	}
	return nil
}

func (h *accHarness) snapshot() accSnapshot {
	s := accSnapshot{at: time.Now(), process: gatherAcc(h.tb, h.process), psBytes: h.psDeliveryBytes.Load()}
	for _, n := range h.nodes {
		s.nodes = append(s.nodes, gatherAcc(h.tb, n.reg))
		in, out := n.host.bytes()
		s.wire = append(s.wire, out)
		s.wireIn = append(s.wireIn, in)
	}
	return s
}

// checkUnion checks AC-C1 on every node: its reserve inside its root equals
// the union of the reserves of the nodes it is connected to (and its own)
// inside that root, and, with rangesync, its Index equals a brute force over
// the reserve (the root summary and every range down to 4 bits below it)
// and its root symbol cache decodes exactly the IDs a probe peer withholds
// (checkCaches).
func (h *accHarness) checkUnion() {
	h.tb.Helper()
	h.checkUnionProbe(true)
}

// checkUnionNoProbe is checkUnion without the probe sessions (checkCaches).
func (h *accHarness) checkUnionNoProbe() {
	h.tb.Helper()
	h.checkUnionProbe(false)
}

func (h *accHarness) checkUnionProbe(probe bool) {
	tb := h.tb
	tb.Helper()

	reserves := make([]map[fingerprint.ID]struct{}, len(h.nodes))
	for _, n := range h.nodes {
		reserves[n.i] = reserveIDs(tb, n.db)
	}
	for _, n := range h.nodes {
		root := n.root()
		union := make(map[fingerprint.ID]struct{})
		mine := make(map[fingerprint.ID]struct{})
		outside := 0
		for id := range reserves[n.i] {
			if root.ContainsID(id) {
				union[id] = struct{}{}
				mine[id] = struct{}{}
			} else {
				outside++
			}
		}
		// no scenario raises a radius (which leaves chunks outside the root
		// until they are evicted), so a reserve holds no chunk outside its
		// root: a node with a nested root (an edge peer, §7.6) synced only
		// the overlapping range (AC-C4)
		if outside != 0 {
			tb.Errorf("node %d (%s): reserve holds %d IDs outside its root %v", n.i, n.mode, outside, root)
		}
		for _, p := range h.nodes {
			if p == n || h.net.conn(n.overlay, p.overlay) == nil {
				continue
			}
			for id := range reserves[p.i] {
				if root.ContainsID(id) {
					union[id] = struct{}{}
				}
			}
		}
		if !maps.Equal(mine, union) {
			tb.Errorf("node %d (%s): reserve holds %d IDs in its root, the union %d (%d missing)", n.i, n.mode, len(mine), len(union), len(minus(union, mine)))
			continue
		}
		if n.svc == nil {
			continue
		}
		st, err := n.svc.LocalStatus()
		if err != nil {
			tb.Errorf("node %d: local status: %v", n.i, err)
			continue
		}
		if !st.Root.Equal(root) {
			tb.Errorf("node %d: index root %v, want %v", n.i, st.Root, root)
			continue
		}
		if want := bruteSummary(mine, root); st.Summary != want {
			tb.Errorf("node %d: index root summary %+v, brute force %+v", n.i, st.Summary, want)
			continue
		}
		for d := uint8(1); d <= 4; d++ {
			for _, r := range root.Children(d) {
				got, err := n.svc.RangeSummary(r)
				if err != nil {
					tb.Errorf("node %d: range summary: %v", n.i, err)
					continue
				}
				if want := bruteSummary(mine, r); got != want {
					tb.Errorf("node %d: index summary of %v: %+v, brute force %+v", n.i, r, got, want)
				}
			}
		}
		if probe {
			h.checkCaches(n)
		}
	}
}

// accProbeWithheld is the number of IDs a probe peer withholds from a node's
// set (checkCaches).
const accProbeWithheld = 40

// checkCaches reconciles a probe peer, holding node n's reserve but
// accProbeWithheld random IDs, with n over a connection that n's topology
// does not see. Both hold more than fingerprint.ScanLimit elements in the
// root, so the session decodes n's symbols from its root cache (a range of
// at most ScanLimit elements is scanned instead): the probe must want exactly
// the withheld IDs, with no corrupt decode.
func (h *accHarness) checkCaches(n *accNode) {
	tb := h.tb
	tb.Helper()

	var chunks []swarm.Chunk
	err := n.db.ReserveIterateChunks(func(ch swarm.Chunk) (bool, error) {
		chunks = append(chunks, ch)
		return false, nil
	})
	if err != nil {
		tb.Fatal(err)
	}
	if len(chunks) <= fingerprint.ScanLimit+accProbeWithheld {
		tb.Fatalf("node %d: %d chunks, too few to probe its root cache", n.i, len(chunks))
	}
	withheld := make(map[fingerprint.ID]struct{})
	perm := rand.New(h.rng).Perm(len(chunks))
	for _, k := range perm[:accProbeWithheld] {
		id, err := chunkIDOf(chunks[k])
		if err != nil {
			tb.Fatal(err)
		}
		withheld[id] = struct{}{}
	}
	held := make([]swarm.Chunk, 0, len(perm)-accProbeWithheld)
	for _, k := range perm[accProbeWithheld:] {
		held = append(held, chunks[k])
	}

	overlay := withPrefix(swarm.NewAddress(h.randBytes(swarm.HashSize)), n.overlay, n.db.StorageRadius())
	store := mockstorer.NewReserve(mockstorer.WithAddrIndex(), mockstorer.WithRadius(n.db.StorageRadius()), mockstorer.WithChunks(held...))
	host := h.net.newHost(overlay, newAccTopology(overlay))
	probe := rangesync.New(host, store, overlay, nil, nil, h.validStamp, log.Noop, rangesync.Options{})
	host.AddProtocol(probe.Protocol())
	defer func() {
		if err := probe.Close(); err != nil {
			tb.Error(err)
		}
	}()
	if err := probe.Start(); err != nil {
		tb.Fatal(err)
	}
	accWait(tb, "the probe's index", 30*time.Second, func() (bool, string) {
		_, err := probe.LocalStatus()
		return err == nil, fmt.Sprint(err)
	})
	if err := h.net.connectQuiet(host, n.host); err != nil {
		tb.Fatal(err)
	}
	defer func() { _ = h.net.disconnect(host, n.host) }()

	ctx := context.Background()
	if err := probe.SendStatus(ctx, n.overlay); err != nil {
		tb.Fatal(err)
	}
	if err := n.svc.SendStatus(ctx, overlay); err != nil {
		tb.Fatal(err)
	}
	accWait(tb, "the node's status at the probe", 10*time.Second, func() (bool, string) {
		_, ok := probe.LastStatus(n.overlay)
		return ok, ""
	})
	probeReg := prometheusRegistry(tb, probe.Metrics()...)
	// the node's root iterations count every session it serves: a session
	// another peer runs with it meanwhile (an edge peer with another root,
	// which a round cannot tell is in sync, T6.1 part 2) would count in, so
	// the probe runs again after one. What the node answered is read at the
	// probe (its parts_received): a session of the node's that was under way
	// when the attempt began is not seen by the stream count, and its parts
	// would count in the node's parts_sent (T6.1 part 2 fix round 1)
	others := func() int64 {
		var sum int64
		for _, p := range h.nodes {
			if p != n {
				k, _ := h.net.pairStats(n.overlay, p.overlay, "rangesync/reconcile")
				sum += k
			}
		}
		return sum
	}
	for attempt := 1; ; attempt++ {
		probe0, iters0, others0 := gatherAcc(tb, probeReg), n.store.rootIters.Load(), others()
		wants, err := probe.Reconcile(ctx, n.overlay)
		if err != nil {
			tb.Fatalf("node %d: probe session: %v", n.i, err)
		}
		pm := gatherAcc(tb, probeReg).since(probe0)
		iters := n.store.rootIters.Load() - iters0
		if others() != others0 && attempt < 3 {
			continue
		}
		if got := idSet(wants); !maps.Equal(got, withheld) {
			tb.Errorf("node %d: the probe wants %d IDs (%d of them withheld), want the %d withheld", n.i, len(got), len(got)-len(minus(got, withheld)), len(withheld))
		}
		if v := pm.value("bee_rangesync_decode_corruption_total"); v != 0 {
			tb.Errorf("node %d: the probe decoded %v corrupt streams", n.i, v)
		}
		// the cache path: the node answered the probe's root Fingerprint
		// with symbols (no split), the probe decoded them before the stream
		// ran out, and the node neither scanned nor listed its root
		sent := pm.byLabel("bee_rangesync_parts_received")
		exhausted := 0.0
		for _, v := range pm.byLabel("bee_rangesync_streams_exhausted_total") {
			exhausted += v
		}
		decoded := pm.hists["bee_rangesync_symbols_per_difference"].count
		if sent["symbols"] == 0 || sent["fingerprint"] != 0 || decoded == 0 || exhausted != 0 || iters != 0 {
			tb.Errorf("node %d: the probe session did not decode the root from its cache: the node sent %v symbol and %v fingerprint parts and iterated its root %d times; the probe decoded %d streams, %v exhausted", n.i, sent["symbols"], sent["fingerprint"], iters, decoded, exhausted)
		}
		return
	}
}

func prometheusRegistry(tb testing.TB, cs ...prometheus.Collector) *prometheus.Registry {
	tb.Helper()

	reg := prometheus.NewRegistry()
	for _, c := range cs {
		if err := reg.Register(c); err != nil {
			tb.Fatal(err)
		}
	}
	return reg
}
