// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package retrieval_test

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	accountingmock "github.com/ethersphere/bee/v2/pkg/accounting/mock"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	pricermock "github.com/ethersphere/bee/v2/pkg/pricer/mock"
	"github.com/ethersphere/bee/v2/pkg/retrieval"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// The retrieval simulator runs the real retrieval code of every node over
// the artificial network of simnet_scaffold_test.go. Each chunk is placed in
// the reserve of full nodes of its neighborhood, without a copy outside of
// it, which is the state that an upload with first-hop memory leaves. The
// origin, a light node, then requests every chunk once.

const simDownloads = 128 // chunks requested at a time

// simReserve is the store of a node.
type simReserve struct {
	mu     sync.Mutex
	chunks map[string]swarm.Chunk
}

func (s *simReserve) Cache() storage.Putter  { return s }
func (s *simReserve) Lookup() storage.Getter { return s }

func (s *simReserve) Put(_ context.Context, ch swarm.Chunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chunks[ch.Address().ByteString()] = ch
	return nil
}

func (s *simReserve) Get(_ context.Context, addr swarm.Address) (swarm.Chunk, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ch, ok := s.chunks[addr.ByteString()]; ok {
		return ch, nil
	}
	return nil, storage.ErrNotFound
}

// simRetrLog counts the debug lines of the origin.
type simRetrLog struct {
	log.Logger
	mu       sync.Mutex
	failed   map[string]int // "failed to get chunk" lines per chunk
	failedPO map[uint8]int  // the same lines by proximity of the peer to the chunk
}

func (l *simRetrLog) V(uint) log.Builder            { return l }
func (l *simRetrLog) WithName(string) log.Builder   { return l }
func (l *simRetrLog) WithValues(...any) log.Builder { return l }
func (l *simRetrLog) Build() log.Logger             { return l }
func (l *simRetrLog) Register() log.Logger          { return l }

func (l *simRetrLog) Debug(msg string, kv ...any) {
	if msg != "failed to get chunk" {
		return
	}
	var (
		chunk swarm.Address
		po    uint8
	)
	for i := 0; i+1 < len(kv); i += 2 {
		switch kv[i] {
		case "chunk_address":
			chunk, _ = kv[i+1].(swarm.Address)
		case "peer_proximity":
			po, _ = kv[i+1].(uint8)
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failed[chunk.ByteString()]++
	l.failedPO[po]++
}

type simRetrResult struct {
	chunks, copies    int
	originPeers       int
	withFailed        int // chunks with at least one failed attempt
	failed            int // failed attempts
	failedPO0         int // failed attempts at peers of proximity 0 to the chunk
	exhausted         int // chunks not retrieved by the first request
	exhaustedAll      int // of those, chunks that used all 32 attempts
	retrievedBySecond int // of the exhausted, retrieved by a second request
	virtual, wall     time.Duration
}

// runRetrieval places p.chunks chunks and downloads them from the origin.
// With allHood false a chunk is held by the closest full node only, with
// allHood true by every full node of its neighborhood. forwarderFirst is the
// share of forwarders that run with the change of the forwarder.
// The models of a peer that is not healthy. They need peerHealth.
const (
	simUnhealthyFlag    = ""        // differs from a healthy peer by the flag only
	simUnhealthySlow    = "slow"    // adds simSlowLatency to every link it is on
	simUnhealthyStall   = "stall"   // gives no answer to simStallShare of its streams
	simUnhealthyMissing = "missing" // does not hold simMissingShare of its chunks

	simSlowLatency  = 300 * time.Millisecond
	simStallShare   = 0.3
	simMissingShare = 0.5
)

func runRetrieval(t *testing.T, p simParams, healthFirst bool, forwarderFirst float64, allHood bool, unhealthy string) simRetrResult {
	t.Helper()

	var (
		g      = newSimGraph(p)
		res    = simRetrResult{chunks: p.chunks, originPeers: len(g.peers[g.origin])}
		l      = &simRetrLog{Logger: log.Noop, failed: make(map[string]int), failedPO: make(map[uint8]int)}
		chunks = make([]swarm.Chunk, p.chunks)
		start  = time.Now()
	)
	for i := range chunks {
		data := make([]byte, 16)
		binary.BigEndian.PutUint64(data, uint64(p.seed))
		binary.BigEndian.PutUint64(data[8:], uint64(i))
		ch, err := cac.New(data)
		if err != nil {
			t.Fatal(err)
		}
		chunks[i] = ch
	}

	synctest.Test(t, func(t *testing.T) {
		var (
			n      = len(g.addrs)
			net    = &simNet{g: g, handlers: map[string][]p2p.HandlerFunc{retrieval.ProtocolName: make([]p2p.HandlerFunc, n)}}
			nodes  = make([]*retrieval.Service, n)
			stores = make([]*simReserve, n)
			pricer = pricermock.NewMockService(10, 10)
			// the same rule as the healthy flag of newSimGraph with peerHealth
			bad = func(i int) bool { return i != g.origin && unit(uint64(p.seed), 3, uint64(i)) >= p.healthy }
		)
		switch unhealthy {
		case simUnhealthySlow:
			net.extra = make([]time.Duration, n)
			for i := range n {
				if bad(i) {
					net.extra[i] = simSlowLatency
				}
			}
		case simUnhealthyStall:
			net.stall = make([]float64, n)
			for i := range n {
				if bad(i) {
					net.stall[i] = simStallShare
				}
			}
		}
		for i := range n {
			var (
				topo   = &simTopology{base: g.addrs[i], peers: g.peers[i], legacy: g.legacy[i], private: g.private[i]}
				logger = log.Noop
				rad    = g.radius[i]
			)
			if i == g.origin {
				logger = l
			}
			stores[i] = &simReserve{chunks: make(map[string]swarm.Chunk)}
			nodes[i] = retrieval.New(g.addrs[i], func() (uint8, error) { return rad, nil }, stores[i],
				&simStreamer{net: net, from: i}, topo, logger, accountingmock.NewAccounting(), pricer, nil, i != g.origin)
			if i != g.origin {
				net.handlers[retrieval.ProtocolName][i] = nodes[i].Protocol().StreamSpecs[0].Handler
				nodes[i].SetForwarderHealthFirst(unit(uint64(p.seed), 7, uint64(i)) >= forwarderFirst)
			}
			nodes[i].SetAnyReach(p.anyReach)
			nodes[i].SetNoHealth(p.noHealth)
		}
		nodes[g.origin].SetHealthFirst(healthFirst)

		for c, ch := range chunks {
			best := -1
			for i := range p.nodes {
				po := swarm.Proximity(g.addrs[i].Bytes(), ch.Address().Bytes())
				if po < g.radius[i] {
					continue
				}
				if unhealthy == simUnhealthyMissing && bad(i) && unit(uint64(p.seed), 6, uint64(i), uint64(c)) < simMissingShare {
					continue
				}
				if allHood {
					_ = stores[i].Put(context.Background(), ch)
					res.copies++
					continue
				}
				if best < 0 {
					best = i
				} else if closer, _ := g.addrs[i].Closer(ch.Address(), g.addrs[best]); closer {
					best = i
				}
			}
			if best >= 0 {
				_ = stores[best].Put(context.Background(), ch)
				res.copies++
			}
		}

		var (
			wg    sync.WaitGroup
			mu    sync.Mutex
			work  = make(chan swarm.Chunk)
			begin = time.Now()
		)
		for range simDownloads {
			wg.Go(func() {
				for ch := range work {
					_, err := nodes[g.origin].RetrieveChunk(context.Background(), ch.Address(), swarm.ZeroAddress)
					if err == nil {
						continue
					}
					_, err2 := nodes[g.origin].RetrieveChunk(context.Background(), ch.Address(), swarm.ZeroAddress)
					mu.Lock()
					res.exhausted++
					if err2 == nil {
						res.retrievedBySecond++
					}
					mu.Unlock()
				}
			})
		}
		for _, ch := range chunks {
			work <- ch
		}
		close(work)
		wg.Wait()
		res.virtual = time.Since(begin)

		// longer than a stream without an answer stays open
		time.Sleep(3 * time.Minute)
		for _, s := range nodes {
			_ = s.Close()
		}
	})
	res.wall = time.Since(start)

	for _, n := range l.failed {
		res.withFailed++
		res.failed += n
		if n >= 32 {
			res.exhaustedAll++
		}
	}
	res.failedPO0 = l.failedPO[0]
	return res
}

func (r simRetrResult) String() string {
	per100 := func(a int) float64 { return 100 * float64(a) / float64(r.chunks) }
	return fmt.Sprintf("chunks %d, copies %d, origin peers %d\n"+
		"  chunks with a failed attempt %.1f%%, failed attempts per 100 chunks %.1f, failed attempts at proximity 0: %d\n"+
		"  chunks not retrieved by the first request %d (with 32 or more failed attempts: %d), retrieved by the second request %d\n"+
		"  download time in the simulation %v, run %v",
		r.chunks, r.copies, r.originPeers, per100(r.withFailed), per100(r.failed), r.failedPO0,
		r.exhausted, r.exhaustedAll, r.retrievedBySecond, r.virtual.Round(time.Second), r.wall.Round(time.Millisecond))
}

// simVariant is the code that the origin and the forwarders run.
type simVariant struct {
	healthFirst    bool    // the origin without the change in closestPeer
	forwarderFirst float64 // share of forwarders with the change of the forwarder
}

// TestSimDownload downloads from a network of v2.8.3 forwarders, without and
// with the change of the origin in closestPeer. Set SIMNET to run it.
func TestSimDownload(t *testing.T) {
	if os.Getenv("SIMNET") == "" {
		t.Skip("set SIMNET to run the simulator")
	}
	nets := map[string]simParams{
		"nodes1000": {seed: 1, nodes: 1000, radius: 8, healthy: 0.32, populated: true, chunks: 24609},
		"nodes4500": {seed: 1, nodes: 4500, radius: 10, healthy: 0.32, populated: true, chunks: 24609},
		// healthy as a property of the peer, the same for every node that looks at it
		"peerhealth": {seed: 1, nodes: 1000, radius: 8, healthy: 0.32, populated: true, peerHealth: true, chunks: 24609},
	}
	for name, p := range nets {
		if os.Getenv("SIMNET") != "all" && os.Getenv("SIMNET") != name {
			continue
		}
		for _, allHood := range []bool{false, true} {
			// origin and forwarders without the change, the origin with it,
			// the origin and the forwarders with it
			for _, v := range []simVariant{{true, 0}, {false, 0}, {false, 1}} {
				t.Run(fmt.Sprintf("%s/allHood=%v/healthFirst=%v/forwarderFirst=%v", name, allHood, v.healthFirst, v.forwarderFirst), func(t *testing.T) {
					t.Logf("\n%s", runRetrieval(t, p, v.healthFirst, v.forwarderFirst, allHood, simUnhealthyFlag))
				})
			}
		}
	}
}

// TestSimDownloadUnhealthy downloads from a network where a peer that is not
// healthy is slow, gives no answer to a part of its streams, or does not hold
// a part of its chunks. Set SIMNET=unhealthy to run it.
func TestSimDownloadUnhealthy(t *testing.T) {
	if os.Getenv("SIMNET") != "unhealthy" {
		t.Skip("set SIMNET=unhealthy to run")
	}
	p := simParams{seed: 1, nodes: 1000, radius: 8, healthy: 0.32, populated: true, peerHealth: true, chunks: 24609}
	modes := []string{simUnhealthySlow, simUnhealthyStall, simUnhealthyMissing}
	if m := os.Getenv("SIMMODE"); m != "" {
		modes = strings.Split(m, ",")
	}
	for _, mode := range modes {
		for _, allHood := range []bool{false, true} {
			for _, v := range []simVariant{{true, 0}, {false, 0}, {false, 1}} {
				t.Run(fmt.Sprintf("%s/allHood=%v/healthFirst=%v/forwarderFirst=%v", mode, allHood, v.healthFirst, v.forwarderFirst), func(t *testing.T) {
					t.Logf("\n%s", runRetrieval(t, p, v.healthFirst, v.forwarderFirst, allHood, mode))
				})
			}
		}
	}
}

// TestSimDownloadMixed downloads from a network where only a share of the
// forwarders runs with the change of the forwarder. The origin runs with the
// change in closestPeer. Set SIMNET=mixed to run it.
func TestSimDownloadMixed(t *testing.T) {
	if os.Getenv("SIMNET") != "mixed" {
		t.Skip("set SIMNET=mixed to run")
	}
	p := simParams{seed: 1, nodes: 1000, radius: 8, healthy: 0.32, populated: true, peerHealth: true, chunks: 24609}
	modes := []string{simUnhealthyFlag, simUnhealthyStall}
	if m, ok := os.LookupEnv("SIMMODE"); ok {
		modes = strings.Split(m, ",")
	}
	for _, mode := range modes {
		for _, allHood := range []bool{false, true} {
			for _, share := range []float64{0.1, 0.25, 0.5, 0.75} {
				t.Run(fmt.Sprintf("mode=%s/allHood=%v/forwarderFirst=%v", mode, allHood, share), func(t *testing.T) {
					t.Logf("\n%s", runRetrieval(t, p, false, share, allHood, mode))
				})
			}
		}
	}
}

// TestSimDownloadReach downloads from networks where a share of the full
// nodes is not reachable. Set SIMNET=reach to run it; SIMREACH selects "pair"
// (health per pair of nodes) or "peer" (health as a property of the peer).
func TestSimDownloadReach(t *testing.T) {
	if os.Getenv("SIMNET") != "reach" {
		t.Skip("set SIMNET=reach to run")
	}
	base := simParams{seed: 1, nodes: 1000, radius: 8, healthy: 0.32, populated: true, chunks: 6000}
	for _, health := range []string{"pair", "peer"} {
		if want := os.Getenv("SIMREACH"); want != "" && want != health {
			continue
		}
		type reach struct {
			share float64
			mode  string
		}
		reaches := make([]reach, 0, 7)
		reaches = append(reaches, reach{0, ""})
		for _, share := range []float64{0.1, 0.3, 0.5} {
			reaches = append(reaches, reach{share, simReachNode}, reach{share, simReachPair})
		}
		for _, r := range reaches {
			p := base
			p.peerHealth = health == "peer"
			p.unreachable, p.reachMode = r.share, r.mode
			for _, allHood := range []bool{false, true} {
				for _, v := range []simVariant{{true, 0}, {false, 0}, {false, 1}} {
					t.Run(fmt.Sprintf("health=%s/unreachable=%v/mode=%s/allHood=%v/healthFirst=%v/forwarderFirst=%v", health, r.share, r.mode, allHood, v.healthFirst, v.forwarderFirst), func(t *testing.T) {
						t.Logf("\n%s", runRetrieval(t, p, v.healthFirst, v.forwarderFirst, allHood, simUnhealthyFlag))
					})
				}
				if r.share > 0 {
					p := p
					p.anyReach = true
					t.Run(fmt.Sprintf("health=%s/unreachable=%v/mode=%s/allHood=%v/anyReach", health, r.share, r.mode, allHood), func(t *testing.T) {
						t.Logf("\n%s", runRetrieval(t, p, false, 1, allHood, simUnhealthyFlag))
					})
				}
			}
		}
	}
}

// TestSimDownloadNoHealth compares the proximity rule of the branch with a
// selection that does not use the health mark at all (the closest reachable
// peer), on every node. Set SIMNET=nohealth to run it.
func TestSimDownloadNoHealth(t *testing.T) {
	if os.Getenv("SIMNET") != "nohealth" {
		t.Skip("set SIMNET=nohealth to run")
	}
	type run struct {
		name string
		p    simParams
		mode string
	}
	pair := simParams{seed: 1, nodes: 1000, radius: 8, healthy: 0.32, populated: true, chunks: 24609}
	peer := pair
	peer.peerHealth = true
	big := pair
	big.nodes, big.radius = 4500, 10
	runs := []run{
		{"pair", pair, simUnhealthyFlag},
		{"pair4500", big, simUnhealthyFlag},
		{"peer", peer, simUnhealthyFlag},
		{"peer-slow", peer, simUnhealthySlow},
		{"peer-stall", peer, simUnhealthyStall},
		{"peer-missing", peer, simUnhealthyMissing},
	}
	for _, r := range runs {
		for _, allHood := range []bool{false, true} {
			for _, noHealth := range []bool{false, true} {
				p := r.p
				p.noHealth = noHealth
				t.Run(fmt.Sprintf("net=%s/allHood=%v/noHealth=%v", r.name, allHood, noHealth), func(t *testing.T) {
					t.Logf("\n%s", runRetrieval(t, p, false, 1, allHood, r.mode))
				})
			}
		}
	}
}

// TestSimDownloadSmoke keeps the simulator working on a small network.
func TestSimDownloadSmoke(t *testing.T) {
	p := simParams{seed: 7, nodes: 150, radius: 5, healthy: 0.32, populated: true, chunks: 500}

	for _, healthFirst := range []bool{true, false} {
		res := runRetrieval(t, p, healthFirst, 0, false, simUnhealthyFlag)
		if res.copies != p.chunks {
			t.Fatalf("placed %d of %d chunks", res.copies, p.chunks)
		}
		if res.exhausted != res.retrievedBySecond {
			t.Fatalf("health first %v: %d chunks not retrieved, %d of them by a second request", healthFirst, res.exhausted, res.retrievedBySecond)
		}
	}
}
