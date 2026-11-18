// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/coreos/go-semver/semver"
	accountingmock "github.com/ethersphere/bee/v2/pkg/accounting/mock"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/postage"
	pricermock "github.com/ethersphere/bee/v2/pkg/pricer/mock"
	"github.com/ethersphere/bee/v2/pkg/pushsync"
	"github.com/ethersphere/bee/v2/pkg/soc"
	stabilmock "github.com/ethersphere/bee/v2/pkg/stabilization/mock"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
)

// The simulator runs the real pushsync code of many nodes in one process.
// What is real: pushsync.PushSync of every node (handler, pushToClosest,
// first-hop memory, receipt signature and check). What is artificial: the
// peer tables (simGraph), the topology driver (simTopology), the transport
// (simNet), the latency of a link, accounting without limits, a stamp that is
// always valid, and the loop that stands for the pusher (simUpload).

const (
	simNetworkID = 1
	// Constants of kademlia that shape a peer table.
	simSaturation     = 8  // a bin with fewer peers than this sets the depth
	simOverSaturation = 18 // a bin does not take more peers than this
	simSuffixBits     = 4  // 16 balanced slots in a bin
	// Constants of the pusher.
	simConcurrentPushes = 128
	simRetryCount       = 6
	// One-way latency of a link, fixed per pair of nodes.
	simMinLatency = 10 * time.Millisecond
	simMaxLatency = 50 * time.Millisecond
)

// simParams describes one run.
type simParams struct {
	seed     int64
	nodes    int     // full nodes; the origin is one more node, a light node
	radius   uint8   // storage radius of the network
	healthy  float64 // share of its peers that a node sees as healthy
	doubling float64 // share of full nodes with radius-1
	// peerHealth makes the healthy flag a property of the peer (the same for
	// every node that looks at it) instead of a property of the pair.
	peerHealth bool
	// upgraded is the share of full nodes that run the forwarder code of this
	// branch. The others run the forwarder code and the ClosestPeer of v2.8.3.
	upgraded  float64
	hopMemory bool // first-hop memory of the origin
	chunks    int
	// populated gives every neighborhood at least one full node. Addresses
	// from random keys leave some neighborhoods empty.
	populated bool
	// unreachable is the share of full nodes that are not reachable, and
	// reachMode says what that means:
	//   simReachNode  every node sees the peer as not reachable, and the
	//                 node itself knows that it is not reachable;
	//   simReachMark  every node sees the peer as not reachable, the node
	//                 itself takes itself for reachable;
	//   simReachPair  a node sees this share of its peers as not reachable,
	//                 every node takes itself for reachable.
	unreachable float64
	reachMode   string
	// noHealth makes retrieval select the closest reachable peer and not
	// look at the health mark (an experiment, download runs only).
	noHealth bool
	// anyReach makes retrieval apply its proximity rule to peers that are
	// not reachable (an experiment, used by the download runs only).
	anyReach bool
}

const (
	simReachNode = "node"
	simReachMark = "mark"
	simReachPair = "pair"
	// simReachSelf: every node sees the peer as reachable, the node itself
	// does not take itself for reachable.
	simReachSelf = "self"
)

func mix(vs ...uint64) uint64 {
	h := uint64(0x9e3779b97f4a7c15)
	for _, v := range vs {
		h ^= v
		h *= 0xbf58476d1ce4e5b9
		h ^= h >> 31
		h *= 0x94d049bb133111eb
		h ^= h >> 29
	}
	return h
}

func unit(vs ...uint64) float64 { return float64(mix(vs...)>>11) / float64(1<<53) }

// simGraph is the artificial network: addresses, keys and peer tables.
type simGraph struct {
	p      simParams
	keys   [][]byte
	addrs  []swarm.Address
	index  map[string]int
	peers  [][]simPeer // sorted by proximity order, shallow first
	radius []uint8
	legacy []bool
	// private nodes do not take themselves for reachable
	private []bool
	origin  int
}

type simPeer struct {
	idx       int
	addr      swarm.Address
	po        uint8
	healthy   bool
	reachable bool
}

func addrBit(a []byte, k int) int { return int(a[k/8]>>(7-k%8)) & 1 }

// slot returns the balanced slot of the address in bin b: the simSuffixBits
// bits that follow bit b.
func slot(a []byte, b int) int {
	s := 0
	for k := b + 1; k <= b+simSuffixBits; k++ {
		s = s<<1 | addrBit(a, k)
	}
	return s
}

// newSimGraph builds the peer tables in the way kademlia does: a node holds
// every node at or above its depth, and in each bin below it one peer per
// balanced slot, as long as the bin of the other side has room. Links are
// symmetric between full nodes. The origin is a light node: it holds peers,
// no node holds it.
func newSimGraph(p simParams) *simGraph {
	rng := rand.New(rand.NewSource(p.seed))
	n := p.nodes + 1
	g := &simGraph{p: p, origin: p.nodes, index: make(map[string]int, n)}
	nonce := make([]byte, 32)
	newKey := func() ([]byte, swarm.Address) {
		key := make([]byte, 32)
		rng.Read(key)
		pk := crypto.Secp256k1PrivateKeyFromBytes(key)
		addr, err := crypto.NewOverlayAddress(pk.PublicKey, simNetworkID, nonce)
		if err != nil {
			panic(err)
		}
		return key, addr
	}
	hoods := make(map[string]bool)
	for i := range n {
		key, addr := newKey()
		// the first nodes go to neighborhoods that have no node yet
		for p.populated && len(hoods) < 1<<p.radius && hoods[simHood(p.radius, addr)] {
			key, addr = newKey()
		}
		hoods[simHood(p.radius, addr)] = true
		g.keys = append(g.keys, key)
		g.addrs = append(g.addrs, addr)
		g.index[addr.ByteString()] = i
		rad := p.radius
		if i != g.origin && p.radius > 0 && unit(uint64(p.seed), 1, uint64(i)) < p.doubling {
			rad--
		}
		g.radius = append(g.radius, rad)
		g.legacy = append(g.legacy, i != g.origin && unit(uint64(p.seed), 2, uint64(i)) >= p.upgraded)
		g.private = append(g.private, i != g.origin && (p.reachMode == simReachNode || p.reachMode == simReachSelf) && unit(uint64(p.seed), 8, uint64(i)) < p.unreachable)
	}

	adj := make([]map[int]uint8, n)
	binCount := make([][swarm.MaxBins]int, n)
	for i := range adj {
		adj[i] = make(map[int]uint8)
	}
	link := func(i, j int, po uint8) {
		if _, ok := adj[i][j]; ok {
			return
		}
		adj[i][j] = po
		binCount[i][po]++
		if i != g.origin {
			adj[j][i] = po
			binCount[j][po]++
		}
	}

	for range 3 {
		for _, i := range rng.Perm(n) {
			var byBin [swarm.MaxBins][]int
			for j := range p.nodes {
				if j == i {
					continue
				}
				po := swarm.Proximity(g.addrs[i].Bytes(), g.addrs[j].Bytes())
				byBin[po] = append(byBin[po], j)
			}
			depth := 0
			for depth < len(byBin) && len(byBin[depth]) >= simSaturation {
				depth++
			}
			for b, cands := range byBin {
				if b >= depth {
					for _, j := range cands {
						link(i, j, uint8(b))
					}
					continue
				}
				var taken [1 << simSuffixBits]bool
				for j, po := range adj[i] {
					if int(po) == b {
						taken[slot(g.addrs[j].Bytes(), b)] = true
					}
				}
				rng.Shuffle(len(cands), func(a, c int) { cands[a], cands[c] = cands[c], cands[a] })
				for _, j := range cands {
					s := slot(g.addrs[j].Bytes(), b)
					// a full node keeps light nodes outside of its bins
					if taken[s] || (i != g.origin && binCount[j][b] >= simOverSaturation) {
						continue
					}
					taken[s] = true
					link(i, j, uint8(b))
				}
			}
		}
	}

	g.peers = make([][]simPeer, n)
	for i := range adj {
		for j, po := range adj[i] {
			h := unit(uint64(p.seed), 3, uint64(i), uint64(j))
			if p.peerHealth {
				h = unit(uint64(p.seed), 3, uint64(j))
			}
			r := unit(uint64(p.seed), 8, uint64(j))
			if p.reachMode == simReachPair {
				r = unit(uint64(p.seed), 8, uint64(i), uint64(j))
			}
			if p.reachMode == simReachSelf {
				r = 1
			}
			g.peers[i] = append(g.peers[i], simPeer{idx: j, addr: g.addrs[j], po: po, healthy: h < p.healthy, reachable: r >= p.unreachable})
		}
		sort.Slice(g.peers[i], func(a, b int) bool {
			x, y := g.peers[i][a], g.peers[i][b]
			if x.po != y.po {
				return x.po < y.po
			}
			return x.idx < y.idx
		})
	}
	return g
}

// simTopology is a static topology.Driver. ClosestPeer and the two iterators
// follow kademlia.Kad over a fixed peer table. Every peer is reachable and
// none is a bootnode. Only the methods that pushsync and retrieval call are
// implemented.
type simTopology struct {
	topology.Driver
	base  swarm.Address
	peers []simPeer
	// legacy selects the ClosestPeer of v2.8.3, which stops at the first bin
	// that is deeper than the proximity of the base to the address.
	legacy bool
	// private makes the node take itself for not reachable
	private bool
}

func (s *simTopology) pass(p simPeer, f topology.Select) bool {
	return (!f.Healthy || p.healthy) && (!f.Reachable || p.reachable)
}

func (s *simTopology) IsReachable() bool { return !s.private }

func (s *simTopology) EachConnectedPeer(f topology.EachPeerFunc, filter topology.Select) error {
	for i := len(s.peers) - 1; i >= 0; i-- {
		if !s.pass(s.peers[i], filter) {
			continue
		}
		if stop, _, err := f(s.peers[i].addr, s.peers[i].po); stop || err != nil {
			return err
		}
	}
	return nil
}

func (s *simTopology) EachConnectedPeerRev(f topology.EachPeerFunc, filter topology.Select) error {
	for _, p := range s.peers {
		if !s.pass(p, filter) {
			continue
		}
		if stop, _, err := f(p.addr, p.po); stop || err != nil {
			return err
		}
	}
	return nil
}

func (s *simTopology) ClosestPeer(addr swarm.Address, includeSelf bool, filter topology.Select, skipPeers ...swarm.Address) (swarm.Address, error) {
	if len(s.peers) == 0 {
		return swarm.Address{}, topology.ErrNotFound
	}
	closest := swarm.ZeroAddress
	if includeSelf {
		closest = s.base
	}
	prox := swarm.Proximity(s.base.Bytes(), addr.Bytes())
	_ = s.EachConnectedPeerRev(func(peer swarm.Address, bin uint8) (bool, bool, error) {
		if swarm.ContainsAddress(skipPeers, peer) {
			return false, false, nil
		}
		if s.legacy && bin > prox && !closest.IsZero() {
			return true, false, nil
		}
		if closest.IsZero() {
			closest = peer
			return false, false, nil
		}
		if closer, _ := peer.Closer(addr, closest); closer {
			closest = peer
		}
		return false, false, nil
	}, filter)
	if closest.IsZero() {
		return swarm.Address{}, topology.ErrNotFound
	}
	if closest.Equal(s.base) {
		return swarm.Address{}, topology.ErrWantSelf
	}
	return closest, nil
}

// simQueue is one direction of a stream: writes never block, reads block
// until there is data or the queue is closed.
type simQueue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	buf    bytes.Buffer
	closed bool
}

func newSimQueue() *simQueue {
	q := &simQueue{}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *simQueue) Write(p []byte) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return 0, io.ErrClosedPipe
	}
	q.cond.Broadcast()
	return q.buf.Write(p)
}

func (q *simQueue) Read(p []byte) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for q.buf.Len() == 0 && !q.closed {
		q.cond.Wait()
	}
	if q.buf.Len() == 0 {
		return 0, io.EOF
	}
	return q.buf.Read(p)
}

func (q *simQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	q.cond.Broadcast()
}

// simStream is one end of a stream in memory. A write takes the latency of
// the link.
type simStream struct {
	in, out *simQueue
	latency time.Duration
}

func (s *simStream) Read(p []byte) (int, error) { return s.in.Read(p) }

func (s *simStream) Write(p []byte) (int, error) {
	time.Sleep(s.latency)
	return s.out.Write(p)
}

func (s *simStream) Close() error {
	s.in.Close()
	s.out.Close()
	return nil
}
func (s *simStream) FullClose() error                  { return s.Close() }
func (s *simStream) Reset() error                      { return s.Close() }
func (s *simStream) Headers() p2p.Headers              { return nil }
func (s *simStream) ResponseHeaders() p2p.Headers      { return nil }
func (s *simStream) Version() (*semver.Version, error) { return nil, nil }

// simNet passes a new stream to the handler of the node it is opened to.
type simNet struct {
	g        *simGraph
	handlers map[string][]p2p.HandlerFunc // protocol name -> handler of each node
	streams  int64
	mu       sync.Mutex
	// extra is added to the latency of a link by the node at either end of it.
	extra []time.Duration
	// stall is the share of the streams to a node that get no answer.
	stall []float64
}

func (n *simNet) latency(from, to int) time.Duration {
	a, b := min(from, to), max(from, to)
	span := uint64(simMaxLatency - simMinLatency)
	lat := simMinLatency + time.Duration(mix(uint64(n.g.p.seed), 4, uint64(a), uint64(b))%span)
	if n.extra != nil {
		lat += n.extra[from] + n.extra[to]
	}
	return lat
}

// simStreamer is the view of one node on the network.
type simStreamer struct {
	net  *simNet
	from int
}

var _ p2p.StreamerDisconnecter = (*simStreamer)(nil)

func (s *simStreamer) NewStream(_ context.Context, address swarm.Address, _ p2p.Headers, protocol, _, _ string) (p2p.Stream, error) {
	to, ok := s.net.g.index[address.ByteString()]
	if !ok || s.net.handlers[protocol] == nil || s.net.handlers[protocol][to] == nil {
		return nil, fmt.Errorf("simnet: no handler for %s at %s", protocol, address)
	}
	s.net.mu.Lock()
	s.net.streams++
	count := s.net.streams
	s.net.mu.Unlock()

	c2s, s2c := newSimQueue(), newSimQueue()
	lat := s.net.latency(s.from, to)
	server := &simStream{in: c2s, out: s2c, latency: lat}
	if s.net.stall != nil && unit(uint64(s.net.g.p.seed), 5, uint64(s.from), uint64(to), uint64(count)) < s.net.stall[to] {
		// no answer: the stream stays open until the caller gives up
		go func() {
			time.Sleep(time.Minute)
			_ = server.Close()
		}()
		return &simStream{in: s2c, out: c2s, latency: lat}, nil
	}
	handler := s.net.handlers[protocol][to]
	peer := p2p.Peer{Address: s.net.g.addrs[s.from], FullNode: s.from != s.net.g.origin}
	go func() {
		_ = handler(context.Background(), peer, server)
		_ = server.Close()
	}()
	return &simStream{in: s2c, out: c2s, latency: lat}, nil
}

func (s *simStreamer) Disconnect(swarm.Address, string) error               { return nil }
func (s *simStreamer) Blocklist(swarm.Address, time.Duration, string) error { return nil }
func (s *simStreamer) NetworkStatus() p2p.NetworkStatus                     { return p2p.NetworkStatusAvailable }

// simStore is the reserve of a node.
type simStore struct {
	mu     sync.Mutex
	chunks map[string]swarm.Chunk
}

func (s *simStore) Report(context.Context, swarm.Chunk, storage.ChunkState) error { return nil }
func (s *simStore) ReservePutter() storage.Putter                                 { return s }

func (s *simStore) Put(_ context.Context, ch swarm.Chunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chunks[ch.Address().ByteString()] = ch
	return nil
}

func (s *simStore) Get(addr swarm.Address) (swarm.Chunk, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, ok := s.chunks[addr.ByteString()]
	return ch, ok
}

// simChunkLog is what the debug lines of the origin say about one chunk.
type simChunkLog struct {
	shallow    int      // "shallow receipt" lines
	shallowPO  []uint8  // proximity of the signer to the chunk, per shallow receipt
	failed     []string // first hop of each "could not push to peer" line
	storer     string   // signer of the valid receipt
	storerPO   uint8
	calls      int // calls of PushChunkToClosest
	notSynced  bool
	otherError string
}

// simLog takes the place of the logger of the origin. The counts are made
// from the same debug lines as the counts of the mainnet uploads.
type simLog struct {
	log.Logger
	mu     sync.Mutex
	chunks map[string]*simChunkLog
}

func newSimLog() *simLog { return &simLog{Logger: log.Noop, chunks: make(map[string]*simChunkLog)} }

func (l *simLog) V(uint) log.Builder          { return l }
func (l *simLog) WithName(string) log.Builder { return l }
func (l *simLog) WithValues(...any) log.Builder {
	return l
}
func (l *simLog) Build() log.Logger    { return l }
func (l *simLog) Register() log.Logger { return l }

func (l *simLog) chunk(addr swarm.Address) *simChunkLog {
	c := l.chunks[addr.ByteString()]
	if c == nil {
		c = &simChunkLog{}
		l.chunks[addr.ByteString()] = c
	}
	return c
}

func (l *simLog) Debug(msg string, kv ...any) {
	if msg != "shallow receipt" && msg != "could not push to peer" && msg != "chunk pushed" {
		return
	}
	var (
		chunk, peer swarm.Address
		po          uint8
	)
	for i := 0; i+1 < len(kv); i += 2 {
		switch kv[i] {
		case "chunk_address":
			chunk, _ = kv[i+1].(swarm.Address)
		case "peer_address":
			peer, _ = kv[i+1].(swarm.Address)
		case "proximity_order":
			po, _ = kv[i+1].(uint8)
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	c := l.chunk(chunk)
	switch msg {
	case "shallow receipt":
		c.shallow++
		c.shallowPO = append(c.shallowPO, po)
	case "could not push to peer":
		c.failed = append(c.failed, peer.ByteString())
	case "chunk pushed":
		c.storer, c.storerPO = peer.ByteString(), po
	}
}

// simResult is the outcome of one run.
type simResult struct {
	p               simParams
	originPeers     int
	originHealthy   int
	meanPeers       float64
	hoodSize        float64 // full nodes per neighborhood of the radius
	chunks          int
	shallow         int
	couldNotSync    int
	otherErrors     int
	maxFailed       int // most "could not push to peer" lines for one chunk
	repeatedHop     int // the same first hop twice for one chunk
	withShallow     int // chunks with at least one shallow receipt
	secondCalls     int // chunks that needed more than one call
	shallowByPO     map[uint8]int
	ordinalShallow  [3]int // by place of the chunk in its neighborhood: 1st, 2nd-6th, 7th and later
	ordinalChunks   [3]int
	storerPOSum     int
	streams         int64
	virtual         time.Duration
	wall, buildWall time.Duration
	heapMiB         uint64
	digest          string
	syncedShallow   int
	syncedMaxFailed int
	syncedRepeated  int
	emptyHoods      int
	storerPrivate   int // synced chunks whose storer does not take itself for reachable
	storerMarked    int // synced chunks whose storer the origin side sees as not reachable
	privateNodes    int
}

func (r simResult) String() string {
	var b strings.Builder
	per100 := func(a, n int) float64 {
		if n == 0 {
			return 0
		}
		return 100 * float64(a) / float64(n)
	}
	fmt.Fprintf(&b, "nodes %d radius %d healthy %.2f doubling %.2f peerHealth %v populated %v upgraded %.2f hopMemory %v seed %d\n",
		r.p.nodes, r.p.radius, r.p.healthy, r.p.doubling, r.p.peerHealth, r.p.populated, r.p.upgraded, r.p.hopMemory, r.p.seed)
	fmt.Fprintf(&b, "  synced chunks only: shallow per 100 chunks %.1f, max failed pushes per chunk %d, repeated first hops %d, empty neighborhoods %d\n",
		per100(r.syncedShallow, r.chunks-r.couldNotSync-r.otherErrors), r.syncedMaxFailed, r.syncedRepeated, r.emptyHoods)
	fmt.Fprintf(&b, "  network: nodes per neighborhood %.1f, mean peers %.0f, origin peers %d (healthy %d)\n", r.hoodSize, r.meanPeers, r.originPeers, r.originHealthy)
	fmt.Fprintf(&b, "  shallow per 100 chunks %.1f (%d / %d), could_not_sync %d, other errors %d\n", per100(r.shallow, r.chunks), r.shallow, r.chunks, r.couldNotSync, r.otherErrors)
	fmt.Fprintf(&b, "  max failed pushes per chunk %d, repeated first hops %d, chunks with a shallow receipt %.1f%%, chunks with a second call %d\n",
		r.maxFailed, r.repeatedHop, per100(r.withShallow, r.chunks), r.secondCalls)
	fmt.Fprintf(&b, "  shallow per 100 by place in neighborhood: 1st %.1f, 2nd-6th %.1f, 7th+ %.1f (chunks %d / %d / %d)\n",
		per100(r.ordinalShallow[0], r.ordinalChunks[0]), per100(r.ordinalShallow[1], r.ordinalChunks[1]), per100(r.ordinalShallow[2], r.ordinalChunks[2]),
		r.ordinalChunks[0], r.ordinalChunks[1], r.ordinalChunks[2])
	pos := make([]int, 0, len(r.shallowByPO))
	for po := range r.shallowByPO {
		pos = append(pos, int(po))
	}
	sort.Ints(pos)
	b.WriteString("  shallow by proximity of the signer:")
	for _, po := range pos {
		fmt.Fprintf(&b, " %d:%d", po, r.shallowByPO[uint8(po)])
	}
	fmt.Fprintf(&b, "\n  mean proximity of the valid storer %.2f, streams %d\n", float64(r.storerPOSum)/float64(max(1, r.chunks-r.couldNotSync-r.otherErrors)), r.streams)
	if r.p.unreachable > 0 {
		fmt.Fprintf(&b, "  unreachable %.2f mode %s: private nodes %d, synced chunks with a private storer %d, with a storer of the unreachable share %d\n",
			r.p.unreachable, r.p.reachMode, r.privateNodes, r.storerPrivate, r.storerMarked)
	}
	fmt.Fprintf(&b, "  upload time in the simulation %v, build %v, run %v, heap after build %d MiB\n  digest %s",
		r.virtual.Round(time.Second), r.buildWall.Round(time.Millisecond), r.wall.Round(time.Millisecond), r.heapMiB, r.digest)
	return b.String()
}

// simNodes builds a pushsync service for every node of the graph. It must be
// called inside a synctest bubble.
func simNodes(t *testing.T, g *simGraph, net *simNet, originLog log.Logger) ([]*pushsync.PushSync, []*simStore) {
	t.Helper()

	var (
		n       = len(g.addrs)
		nodes   = make([]*pushsync.PushSync, n)
		stores  = make([]*simStore, n)
		pricer  = pricermock.NewMockService(10, 10)
		stable  = stabilmock.NewSubscriber(true)
		noStamp = func(ch swarm.Chunk) (swarm.Chunk, error) { return ch, nil }
		nonce   = make([]byte, 32)
	)
	net.handlers[pushsync.ProtocolName] = make([]p2p.HandlerFunc, n)
	for i := range n {
		var (
			signer = crypto.NewDefaultSigner(crypto.Secp256k1PrivateKeyFromBytes(g.keys[i]))
			topo   = &simTopology{base: g.addrs[i], peers: g.peers[i], legacy: g.legacy[i], private: g.private[i]}
			logger = log.Noop
			rad    = g.radius[i]
		)
		if i == g.origin {
			logger = originLog
		}
		stores[i] = &simStore{chunks: make(map[string]swarm.Chunk)}
		ps := pushsync.New(g.addrs[i], simNetworkID, nonce, &simStreamer{net: net, from: i}, stores[i],
			func() (uint8, error) { return rad, nil }, topo, i != g.origin,
			func(swarm.Chunk) {}, func(*soc.SOC) {}, noStamp, logger,
			accountingmock.NewAccounting(), pricer, signer, nil, stable)
		simSetLegacyForwarder(t, ps, g.legacy[i])
		nodes[i] = ps
		if i != g.origin {
			net.handlers[pushsync.ProtocolName][i] = ps.Protocol().StreamSpecs[0].Handler
		}
	}
	return nodes, stores
}

// simChunks returns valid content addressed chunks with one fixed stamp.
func simChunks(seed int64, n int) []swarm.Chunk {
	stamp := postage.NewStamp(make([]byte, 32), make([]byte, 8), make([]byte, 8), make([]byte, 65))
	chunks := make([]swarm.Chunk, n)
	for i := range chunks {
		data := make([]byte, 16)
		binary.BigEndian.PutUint64(data, uint64(seed))
		binary.BigEndian.PutUint64(data[8:], uint64(i))
		ch, err := cac.New(data)
		if err != nil {
			panic(err)
		}
		chunks[i] = ch.WithStamp(stamp)
	}
	return chunks
}

// simUpload stands for the pusher: simConcurrentPushes chunks at a time, and
// for each chunk up to simRetryCount calls that end with a shallow receipt.
func simUpload(origin *pushsync.PushSync, chunks []swarm.Chunk, l *simLog) {
	var (
		wg   sync.WaitGroup
		work = make(chan swarm.Chunk)
	)
	for range simConcurrentPushes {
		wg.Go(func() {
			for ch := range work {
				var tries, others, calls int
				var notSynced bool
				var other string
			push:
				for {
					calls++
					_, err := origin.PushChunkToClosest(context.Background(), ch)
					switch {
					case err == nil:
						break push
					case errors.Is(err, pushsync.ErrShallowReceipt), errors.Is(err, pushsync.ErrOutOfDepthStoring):
						if tries++; tries >= simRetryCount {
							notSynced = true
							break push
						}
					case errors.Is(err, topology.ErrNotFound):
						time.Sleep(5 * time.Second)
					default:
						// the pusher repeats without a limit; the simulator stops
						if others++; others >= 20 {
							other = err.Error()
							break push
						}
					}
				}
				l.mu.Lock()
				c := l.chunk(ch.Address())
				c.calls, c.notSynced, c.otherError = calls, notSynced, other
				l.mu.Unlock()
			}
		})
	}
	for _, ch := range chunks {
		work <- ch
	}
	close(work)
	wg.Wait()
}

// simGraphs keeps the graphs of earlier runs: runs that differ only in the
// origin share one graph.
var simGraphs sync.Map

func simGraphFor(p simParams) *simGraph {
	key := p
	key.hopMemory, key.chunks = false, 0
	if g, ok := simGraphs.Load(key); ok {
		return g.(*simGraph)
	}
	g := newSimGraph(p)
	simGraphs.Store(key, g)
	return g
}

// runSim uploads p.chunks chunks from the origin and returns what the origin saw.
func runSim(t *testing.T, p simParams) (simResult, []*simStore, []swarm.Chunk, *simGraph) {
	t.Helper()

	var (
		res    = simResult{p: p, shallowByPO: make(map[uint8]int)}
		start  = time.Now()
		g      = simGraphFor(p)
		chunks = simChunks(p.seed, p.chunks)
		stores []*simStore
	)
	res.buildWall = time.Since(start)

	for i, peers := range g.peers {
		if i == g.origin {
			res.originPeers = len(peers)
			for _, peer := range peers {
				if peer.healthy {
					res.originHealthy++
				}
			}
			continue
		}
		res.meanPeers += float64(len(peers)) / float64(p.nodes)
	}
	res.hoodSize = float64(p.nodes) / float64(uint64(1)<<p.radius)
	full := make(map[string]bool)
	for i, addr := range g.addrs {
		if i != g.origin {
			full[simHood(p.radius, addr)] = true
		}
	}
	res.emptyHoods = 1<<p.radius - len(full)
	for _, private := range g.private {
		if private {
			res.privateNodes++
		}
	}

	l := newSimLog()
	synctest.Test(t, func(t *testing.T) {
		net := &simNet{g: g, handlers: make(map[string][]p2p.HandlerFunc)}
		nodes, st := simNodes(t, g, net, l)
		stores = st
		simSetNoHopMemory(t, nodes[g.origin], !p.hopMemory)

		var m runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m)
		res.heapMiB = m.HeapAlloc >> 20

		begin := time.Now()
		simUpload(nodes[g.origin], chunks, l)
		res.virtual = time.Since(begin)
		res.streams = net.streams

		// pushes that lost against a faster one are still in flight
		time.Sleep(time.Minute)
		for _, ps := range nodes {
			_ = ps.Close()
		}
	})
	res.wall = time.Since(start) - res.buildWall

	var (
		digest = sha256.New()
		hoods  = make(map[string]int)
	)
	res.chunks = len(chunks)
	for _, ch := range chunks {
		c := l.chunk(ch.Address())
		res.shallow += c.shallow
		for _, po := range c.shallowPO {
			res.shallowByPO[po]++
		}
		if c.shallow > 0 {
			res.withShallow++
		}
		if c.calls > 1 {
			res.secondCalls++
		}
		switch {
		case c.otherError != "":
			res.otherErrors++
		case c.notSynced:
			res.couldNotSync++
		default:
			res.storerPOSum += int(c.storerPO)
			if i, ok := g.index[c.storer]; ok {
				if g.private[i] {
					res.storerPrivate++
				}
				if p.reachMode != simReachPair && p.reachMode != simReachSelf && unit(uint64(p.seed), 8, uint64(i)) < p.unreachable {
					res.storerMarked++
				}
			}
		}
		res.maxFailed = max(res.maxFailed, len(c.failed))
		seen := make(map[string]bool)
		repeated := 0
		for _, peer := range c.failed {
			if seen[peer] {
				repeated++
			}
			seen[peer] = true
		}
		res.repeatedHop += repeated
		if !c.notSynced && c.otherError == "" {
			res.syncedShallow += c.shallow
			res.syncedMaxFailed = max(res.syncedMaxFailed, len(c.failed))
			res.syncedRepeated += repeated
		}

		hood := simHood(p.radius, ch.Address())
		place := 2
		switch n := hoods[hood]; {
		case n == 0:
			place = 0
		case n < 6:
			place = 1
		}
		hoods[hood]++
		res.ordinalChunks[place]++
		res.ordinalShallow[place] += c.shallow

		failed := append([]string{}, c.failed...)
		sort.Strings(failed)
		fmt.Fprintf(digest, "%x|%d|%x|%x|%d|%v|%s\n", ch.Address().Bytes(), c.shallow, strings.Join(failed, ""), c.storer, c.calls, c.notSynced, c.otherError)
	}
	res.digest = hex.EncodeToString(digest.Sum(nil))[:16]
	return res, stores, chunks, g
}

// simHood returns the first rad bits of the address.
func simHood(rad uint8, addr swarm.Address) string {
	b := append([]byte{}, addr.Bytes()[:rad/8+1]...)
	b[rad/8] &= 0xff << (8 - rad%8)
	return string(b)
}
