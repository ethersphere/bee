// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sim

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/cmd/pullsim/internal/event"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	kadmock "github.com/ethersphere/bee/v2/pkg/topology/kademlia/mock"
)

var _ event.Provider = (*Network)(nil)

// defaultSettleAfter is the quiescence window after which a batch that has
// received no further puts is considered done propagating.
const defaultSettleAfter = 3 * time.Second

// sweepInterval is how often quiescent batches are checked for completion.
const sweepInterval = 250 * time.Millisecond

// Config parameterizes a simulated network.
type Config struct {
	Nodes       int
	Bins        uint8
	Topology    Topology
	Degree      int
	Radius      uint8
	Latency     time.Duration
	MaxPage     uint64
	Clusters    int
	Seed        int64
	SettleAfter time.Duration
}

func (c *Config) applyDefaults() {
	if c.Bins == 0 {
		c.Bins = 8
	}
	if c.MaxPage == 0 {
		c.MaxPage = 64
	}
	if c.Clusters < 1 {
		c.Clusters = 1
	}
	if c.Topology == "" {
		c.Topology = TopologyFull
	}
	if c.Degree == 0 {
		c.Degree = 6
	}
	if c.SettleAfter <= 0 {
		c.SettleAfter = defaultSettleAfter
	}
}

func (c Config) validate() error {
	if c.Nodes < 2 {
		return fmt.Errorf("need at least 2 nodes, got %d", c.Nodes)
	}
	if c.Radius >= c.Bins {
		return fmt.Errorf("radius %d must be < bins %d", c.Radius, c.Bins)
	}
	if _, err := ParseTopology(string(c.Topology)); err != nil {
		return err
	}
	return nil
}

// Network is a set of synthetic nodes connected in memory. It implements
// event.Provider so the bus can pull authoritative snapshots.
type Network struct {
	cfg    Config
	logger log.Logger

	nodes    []*Node
	adj      [][]int
	poMatrix [][]uint8
	idx      map[string]int

	ctx    context.Context
	cancel context.CancelFunc

	// tracker holds its own mutex and never calls back into the Network, so it
	// is kept outside the mu-guarded block.
	tracker *tracker
	sweepWG sync.WaitGroup

	mu          sync.Mutex
	bus         *event.Bus
	radius      uint8
	nodeTraced  []bool
	traced      map[string]bool
	injectSeq   int64
	injectStops []context.CancelFunc
}

// BuildNetwork constructs a network (nodes, topology, protocol wiring) without
// starting the pullers. Call SetBus (optional) then Start.
func BuildNetwork(cfg Config, logger log.Logger) (*Network, error) {
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if cfg.Nodes < 10 || cfg.Nodes > 50 {
		logger.Warning("node count outside the recommended 10-50 range", "nodes", cfg.Nodes)
	}

	n := &Network{
		cfg:        cfg,
		logger:     logger,
		radius:     cfg.Radius,
		idx:        make(map[string]int, cfg.Nodes),
		traced:     make(map[string]bool),
		nodeTraced: make([]bool, cfg.Nodes),
		tracker:    newTracker(cfg.SettleAfter, time.Now),
	}

	epoch := uint64(time.Now().UnixNano())
	rng := rand.New(rand.NewSource(cfg.Seed))

	addrs := n.placeAddresses(rng)
	sort.Slice(addrs, func(i, j int) bool { return addrs[i].Compare(addrs[j]) < 0 })
	for i, a := range addrs {
		n.idx[a.ByteString()] = i
	}

	// Shared wire hooks resolve node identity from addresses.
	hooks := TransportHooks{
		OnMsg: func(m MsgEvent) {
			n.publish(event.Msg{
				Client: n.index(m.Client), Server: n.index(m.Server),
				Proto: m.Proto, Stream: m.Stream, StreamID: m.StreamID,
				Dir: m.Dir, Type: m.Type, Bin: m.Bin, N: m.N,
				Addr: m.Addr, HasAddr: len(m.Addr.Bytes()) == swarm.HashSize,
				Traced: n.isTraced(m.Addr),
			})
		},
		OnStream: func(s StreamEvent) {
			n.publish(event.StreamLC{
				Client: n.index(s.Client), Server: n.index(s.Server),
				Stream: s.Stream, StreamID: s.StreamID,
				Open: s.Phase == StreamOpen, BytesC2S: s.BytesC2S, BytesS2C: s.BytesS2C,
			})
		},
	}

	n.nodes = make([]*Node, cfg.Nodes)
	for i, a := range addrs {
		n.nodes[i] = newNode(i, a, cfg.Bins, cfg.Radius, epoch, cfg.Latency, cfg.MaxPage, logger,
			n.putHook(i), hooks)
	}

	// Topology and per-edge proximity orders.
	n.adj = buildAdjacency(cfg.Topology, addrs, cfg.Degree, rng)
	n.poMatrix = make([][]uint8, cfg.Nodes)
	for i := range n.poMatrix {
		n.poMatrix[i] = make([]uint8, cfg.Nodes)
		for j := range n.poMatrix[i] {
			n.poMatrix[i][j] = edgePO(addrs[i], addrs[j], cfg.Bins)
		}
	}

	// Directed handler wiring: node i can dial each peer j's protocol.
	for i, peers := range n.adj {
		for _, j := range peers {
			n.nodes[i].Transport.SetHandler(addrs[j], n.nodes[j].Syncer.Protocol())
		}
	}

	// Kademlia peers + puller per node.
	for i, peers := range n.adj {
		tuples := make([]kadmock.AddrTuple, 0, len(peers))
		for _, j := range peers {
			tuples = append(tuples, kadmock.AddrTuple{Addr: addrs[j], PO: n.poMatrix[i][j]})
		}
		n.nodes[i].attachPuller(cfg.Bins, logger, tuples, n.onSync(i))
	}

	return n, nil
}

// placeAddresses generates node base addresses, optionally clustered so
// intra-cluster proximities are high (crisp neighborhoods).
func (n *Network) placeAddresses(rng *rand.Rand) []swarm.Address {
	addrs := make([]swarm.Address, n.cfg.Nodes)
	if n.cfg.Clusters <= 1 {
		for i := range addrs {
			addrs[i] = randAddr(rng)
		}
		return addrs
	}
	bases := make([]swarm.Address, n.cfg.Clusters)
	for i := range bases {
		bases[i] = randAddr(rng)
	}
	clusterProx := int(n.cfg.Bins) + 4
	for i := range addrs {
		addrs[i] = randAddrAt(rng, bases[i%n.cfg.Clusters], clusterProx)
	}
	return addrs
}

// Config returns the network configuration.
func (n *Network) Config() Config { return n.cfg }

// Nodes returns the node slice (read-only use).
func (n *Network) Nodes() []*Node { return n.nodes }

// SetBus attaches the event bus used for observability. May be nil.
func (n *Network) SetBus(b *event.Bus) {
	n.mu.Lock()
	n.bus = b
	n.mu.Unlock()
}

func (n *Network) publish(ev any) {
	n.mu.Lock()
	b := n.bus
	n.mu.Unlock()
	if b != nil {
		b.Publish(ev)
	}
}

func (n *Network) index(a swarm.Address) int {
	if i, ok := n.idx[a.ByteString()]; ok {
		return i
	}
	return -1
}

func (n *Network) putHook(idx int) func(PutEvent) {
	return func(pe PutEvent) {
		// Fold into the batch tracker first, holding no Network lock: the
		// tracker has its own mutex and never calls back in here.
		n.tracker.observePut(idx, pe.Address, pe.Source)

		traced := n.isTraced(pe.Address)
		if traced {
			n.mu.Lock()
			n.nodeTraced[idx] = true
			n.mu.Unlock()
		}
		n.publish(event.Put{
			Node: idx, Addr: pe.Address, Bin: pe.Bin, BinID: pe.BinID,
			Source: string(pe.Source), ReserveSize: pe.ReserveSize, Traced: traced,
		})
	}
}

func (n *Network) onSync(idx int) func(SyncEvent) {
	return func(se SyncEvent) {
		if se.Err != nil {
			return // skip cancellation noise
		}
		n.publish(event.Sync{
			From: idx, Peer: n.index(se.Peer), Bin: se.Bin,
			Start: se.Start, Topmost: se.Topmost, Count: se.Count,
			DurMs: se.Dur.Milliseconds(),
		})
	}
}

func (n *Network) isTraced(a swarm.Address) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.traced[a.ByteString()]
}

func (n *Network) markTraced(addrs []swarm.Address) {
	n.mu.Lock()
	for _, a := range addrs {
		n.traced[a.ByteString()] = true
	}
	n.mu.Unlock()
}

// Start launches every node's puller and the batch sweep loop.
func (n *Network) Start(ctx context.Context) {
	n.ctx, n.cancel = context.WithCancel(ctx)
	for _, nd := range n.nodes {
		nd.Puller.Start(n.ctx)
	}
	n.sweepWG.Add(1)
	go n.sweepLoop(n.ctx)
}

// sweepLoop settles quiescent batches and publishes them. It exits with the
// network context.
func (n *Network) sweepLoop(ctx context.Context) {
	defer n.sweepWG.Done()
	tick := time.NewTicker(sweepInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			for _, b := range n.tracker.sweep() {
				n.publish(event.Batch{BatchSnap: batchSnap(b)})
			}
		}
	}
}

// Close stops the network, ordered to avoid the Syncer's 5s handler wait:
// pullers first (cancel client streams), then transports (cancel parked
// handler contexts), then syncers, then reserves.
func (n *Network) Close() {
	n.StopInject()
	if n.cancel != nil {
		n.cancel()
	}
	// Wake anything blocked in WaitBatch before waiting on the sweep loop:
	// after the context is cancelled no batch can ever settle, so an unsettled
	// batch's done channel would otherwise never close.
	n.tracker.stop()
	n.sweepWG.Wait()
	closeConcurrent(n.nodes, func(nd *Node) { _ = nd.Puller.Close() })
	closeConcurrent(n.nodes, func(nd *Node) { _ = nd.Transport.Close() })
	closeConcurrent(n.nodes, func(nd *Node) { _ = nd.Syncer.Close() })
	closeConcurrent(n.nodes, func(nd *Node) { _ = nd.Reserve.Close() })
}

func closeConcurrent(nodes []*Node, fn func(*Node)) {
	var wg sync.WaitGroup
	for _, nd := range nodes {
		if nd == nil {
			continue
		}
		wg.Add(1)
		go func(nd *Node) {
			defer wg.Done()
			fn(nd)
		}(nd)
	}
	wg.Wait()
}

// SetRadius applies a new storage radius to every node and triggers each
// kademlia so the pullers re-evaluate promptly.
func (n *Network) SetRadius(r uint8) {
	if r >= n.cfg.Bins {
		r = n.cfg.Bins - 1
	}
	n.mu.Lock()
	n.radius = r
	n.mu.Unlock()
	for i, nd := range n.nodes {
		nd.Reserve.SetRadius(r)
		nd.Kad.Trigger()
		n.publish(event.Radius{Node: i, Radius: r})
	}
}

// Radius returns the current global storage radius.
func (n *Network) Radius() uint8 {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.radius
}

// Inject mines count chunks near node's base at proximity >= minPO and stores
// them, optionally streaming at ratePerSec (<=0 or count==1 stores
// immediately). count==1 marks the chunk as traced. It returns the propagation
// batch ID (see WaitBatch) and the addresses.
func (n *Network) Inject(node, count int, ratePerSec float64, minPO uint8) (int, []swarm.Address, error) {
	if node < 0 || node >= len(n.nodes) {
		return 0, nil, fmt.Errorf("node index %d out of range", node)
	}
	if count < 1 {
		return 0, nil, fmt.Errorf("count must be >= 1")
	}
	nd := n.nodes[node]

	n.mu.Lock()
	n.injectSeq++
	seed := n.cfg.Seed ^ (n.injectSeq * 2654435761) ^ int64(node)
	n.mu.Unlock()
	rng := rand.New(rand.NewSource(seed))

	chunks := make([]swarm.Chunk, count)
	addrs := make([]swarm.Address, count)
	for i := 0; i < count; i++ {
		ch := chunkAt(rng, nd.Addr, minPO)
		chunks[i] = ch
		addrs[i] = ch.Address()
	}

	batchID := n.tracker.register(node, addrs)

	traced := count == 1
	if traced {
		n.markTraced(addrs)
	}
	n.publish(event.Inject{Node: node, Count: count, Addrs: addrs, Traced: traced})

	if ratePerSec <= 0 || count == 1 {
		for _, ch := range chunks {
			_ = nd.Reserve.Inject(ch)
		}
		return batchID, addrs, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	n.mu.Lock()
	n.injectStops = append(n.injectStops, cancel)
	n.mu.Unlock()

	interval := time.Duration(float64(time.Second) / ratePerSec)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		i := 0
		for i < len(chunks) {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = nd.Reserve.Inject(chunks[i])
				i++
			}
		}
	}()

	return batchID, addrs, nil
}

// Batches returns the retained propagation batches, oldest first.
func (n *Network) Batches() []Batch { return n.tracker.list() }

// Batch returns a single batch by ID.
func (n *Network) Batch(id int) (Batch, bool) { return n.tracker.get(id) }

// ErrNetworkClosed is returned by WaitBatch when the network shut down before
// the batch settled. The partial batch is returned alongside it.
var ErrNetworkClosed = errors.New("network closed before batch settled")

// WaitBatch blocks until the batch stops propagating, the context is done, or
// the network shuts down. On any error it still returns the batch as observed
// so far, so a caller that timed out can report partial results.
func (n *Network) WaitBatch(ctx context.Context, id int) (Batch, error) {
	done := n.tracker.wait(id)
	select {
	case <-done:
		b, ok := n.tracker.get(id)
		if !ok {
			return Batch{}, fmt.Errorf("batch %d no longer retained", id)
		}
		if !b.Settled {
			// Woken by Close, not by settling.
			return b, ErrNetworkClosed
		}
		return b, nil
	case <-ctx.Done():
		b, _ := n.tracker.get(id)
		return b, ctx.Err()
	}
}

// batchSnap converts a tracked batch to its wire form.
func batchSnap(b Batch) event.BatchSnap {
	return event.BatchSnap{
		ID: b.ID, Origin: b.Origin, Chunks: b.Chunks,
		Replicas: b.Replicas, NodesReached: b.NodesReached, Settled: b.Settled,
		LateReplicas: b.LateReplicas,
		SpanMs:       b.Metrics.SpanMs, InjectMs: b.Metrics.InjectMs, TailMs: b.Metrics.TailMs,
		PerDeliveryP50Ms: b.Metrics.PerDeliveryP50Ms,
		PerDeliveryP95Ms: b.Metrics.PerDeliveryP95Ms,
		PerDeliveryMaxMs: b.Metrics.PerDeliveryMaxMs,
	}
}

// StopInject cancels all in-progress streaming injects.
func (n *Network) StopInject() {
	n.mu.Lock()
	stops := n.injectStops
	n.injectStops = nil
	n.mu.Unlock()
	for _, cancel := range stops {
		cancel()
	}
}

// Snapshot implements event.Provider.
func (n *Network) Snapshot() event.Snapshot {
	n.mu.Lock()
	defer n.mu.Unlock()

	nodes := make([]event.NodeSnap, len(n.nodes))
	radii := make([]uint8, len(n.nodes))
	total := 0
	for i, nd := range n.nodes {
		size := nd.Reserve.ReserveSize()
		total += size
		radii[i] = nd.Reserve.StorageRadius()
		nodes[i] = event.NodeSnap{
			Index:       i,
			AddrPrefix:  nd.Addr.String()[:8],
			Angle:       angleOf(nd.Addr),
			Radius:      radii[i],
			ReserveSize: size,
			BinCounts:   nd.Reserve.BinCounts(),
			HasTraced:   n.nodeTraced[i],
		}
	}

	edges := make([]event.EdgeSnap, 0)
	for i, peers := range n.adj {
		for _, j := range peers {
			if i < j {
				po := n.poMatrix[i][j]
				edges = append(edges, event.EdgeSnap{
					A: i, B: j, PO: po,
					Mode: edgeMode(po, radii[i], radii[j]),
				})
			}
		}
	}

	tracked := n.Batches()
	batches := make([]event.BatchSnap, len(tracked))
	for i, b := range tracked {
		batches[i] = batchSnap(b)
	}

	return event.Snapshot{
		Nodes:   nodes,
		Edges:   edges,
		Batches: batches,
		Stats:   event.Stats{Chunks: total, Goroutines: runtime.NumGoroutine()},
	}
}

func angleOf(a swarm.Address) float64 {
	b := a.Bytes()
	if len(b) < 2 {
		return 0
	}
	v := binary.BigEndian.Uint16(b[:2])
	return float64(v) / 65536.0 * 2 * math.Pi
}

func modeRank(po, radius uint8) int {
	switch {
	case po >= radius:
		return 2
	case radius-po <= 2:
		return 1
	default:
		return 0
	}
}

func edgeMode(po, ra, rb uint8) string {
	r := modeRank(po, ra)
	if x := modeRank(po, rb); x > r {
		r = x
	}
	switch r {
	case 2:
		return event.ModeFull
	case 1:
		return event.ModePOOnly
	default:
		return event.ModeIdle
	}
}
