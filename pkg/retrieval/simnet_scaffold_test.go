// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package retrieval_test

// This file is a copy of the network part of pkg/pushsync/simnet_test.go
// (peer tables, static topology driver, transport in memory). A test package
// cannot import another test package, so the spike keeps two copies.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/coreos/go-semver/semver"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
)

const (
	simNetworkID = 1
	// Constants of kademlia that shape a peer table.
	simSaturation     = 8  // a bin with fewer peers than this sets the depth
	simOverSaturation = 18 // a bin does not take more peers than this
	simSuffixBits     = 4  // 16 balanced slots in a bin
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
	upgraded float64
	chunks   int
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
	noHealth bool //nolint:unused // used by the copy of this file in pkg/retrieval
	// anyReach makes retrieval apply its proximity rule to peers that are
	// not reachable (an experiment, used by the download runs only).
	anyReach bool //nolint:unused // used by the copy of this file in pkg/retrieval
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

// simHood returns the first rad bits of the address.
func simHood(rad uint8, addr swarm.Address) string {
	b := append([]byte{}, addr.Bytes()[:rad/8+1]...)
	b[rad/8] &= 0xff << (8 - rad%8)
	return string(b)
}
