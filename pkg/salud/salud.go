// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package salud monitors the connected peers, calculates certain thresholds, and marks peers as unhealthy that
// fall short of the thresholds to maintain network salud (health).
package salud

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/status"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"go.uber.org/atomic"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "salud"

const (
	wakeup                 = time.Minute * 5
	requestTimeout         = time.Second * 10
	DefaultMinPeersPerBin  = 4
	DefaultDurPercentile   = 0.4 // consider 40% as healthy, lower percentile = stricter duration check
	DefaultConnsPercentile = 0.8 // consider 80% as healthy, lower percentile = stricter conns check
)

type topologyDriver interface {
	UpdatePeerHealth(peer swarm.Address, health bool, dur time.Duration)
	topology.PeerIterator
}

type peerStatus interface {
	PeerSnapshot(ctx context.Context, peer swarm.Address) (*status.Snapshot, error)
}

type service struct {
	wg            sync.WaitGroup
	quit          chan struct{}
	logger        log.Logger
	topology      topologyDriver
	status        peerStatus
	metrics       metrics
	isSelfHealthy *atomic.Bool
	reserve       storer.RadiusChecker

	radiusSubsMtx sync.Mutex
	radiusC       []chan uint8
}

func New(
	status peerStatus,
	topology topologyDriver,
	reserve storer.RadiusChecker,
	logger log.Logger,
	warmup time.Duration,
	mode string,
	minPeersPerbin int,
	durPercentile float64,
	connsPercentile float64,
) *service {

	metrics := newMetrics()

	s := &service{
		quit:          make(chan struct{}),
		logger:        logger.WithName(loggerName).Register(),
		status:        status,
		topology:      topology,
		metrics:       metrics,
		isSelfHealthy: atomic.NewBool(true),
		reserve:       reserve,
	}

	s.wg.Add(1)
	go s.worker(warmup, mode, minPeersPerbin, durPercentile, connsPercentile)

	return s

}

func (s *service) worker(warmup time.Duration, mode string, minPeersPerbin int, durPercentile float64, connsPercentile float64) {
	defer s.wg.Done()

	select {
	case <-s.quit:
		return
	case <-time.After(warmup):
	}

	for {

		s.salud(mode, minPeersPerbin, durPercentile, connsPercentile)

		select {
		case <-s.quit:
			return
		case <-time.After(wakeup):
		}
	}
}

func (s *service) Close() error {
	close(s.quit)
	s.wg.Wait()
	return nil
}

type peer struct {
	status   *status.Snapshot
	dur      time.Duration
	addr     swarm.Address
	bin      uint8
	neighbor bool
}

// salud acquires the status snapshot of every peer and computes an nth percentile of response duration and connected
// per count, the most common storage radius, and the batch commitment, and based on these values, marks peers as unhealhy that fall beyond
// the allowed thresholds.
func (s *service) salud(mode string, minPeersPerbin int, durPercentile float64, connsPercentile float64) {

	var (
		mtx      sync.Mutex
		wg       sync.WaitGroup
		totaldur float64
		peers    []peer
		bins     [swarm.MaxBins]int
	)

	_ = s.topology.EachConnectedPeer(func(addr swarm.Address, bin uint8) (stop bool, jumpToNext bool, err error) {
		wg.Add(1)
		go func() {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
			defer cancel()

			start := time.Now()
			snapshot, err := s.status.PeerSnapshot(ctx, addr)
			dur := time.Since(start)

			if err != nil {
				s.topology.UpdatePeerHealth(addr, false, dur)
				return
			}

			if snapshot.BeeMode != mode {
				return
			}

			mtx.Lock()
			bins[bin]++
			totaldur += dur.Seconds()
			peers = append(peers, peer{snapshot, dur, addr, bin, s.reserve.IsWithinStorageRadius(addr)})
			mtx.Unlock()
		}()
		return false, false, nil
	}, topology.Select{})

	wg.Wait()

	if len(peers) == 0 {
		return
	}

	networkRadius, nHoodRadius := s.committedDepth(peers)
	avgDur := totaldur / float64(len(peers))
	pDur := percentileDur(peers, durPercentile)
	pConns := percentileConns(peers, connsPercentile)
	commitment := commitment(peers)

	s.metrics.AvgDur.Set(avgDur)
	s.metrics.PDur.Set(pDur)
	s.metrics.PConns.Set(float64(pConns))
	s.metrics.NetworkRadius.Set(float64(networkRadius))
	s.metrics.NeighborhoodRadius.Set(float64(nHoodRadius))
	s.metrics.Commitment.Set(float64(commitment))

	s.logger.Debug("computed", "avg_dur", avgDur, "pDur", pDur, "pConns", pConns, "network_radius", networkRadius, "neighborhood_radius", nHoodRadius, "batch_commitment", commitment)

	// sort peers by duration, highest first to give priority to the fastest peers
	sort.Slice(peers, func(i, j int) bool {
		return peers[i].dur > peers[j].dur // descending
	})

	for _, peer := range peers {

		var healthy bool

		// every bin should have at least some peers, healthy or not
		if bins[peer.bin] <= minPeersPerbin {
			s.metrics.Healthy.Inc()
			s.topology.UpdatePeerHealth(peer.addr, true, peer.dur)
			continue
		}

		if networkRadius > 0 && peer.status.CommittedDepth < uint32(networkRadius-2) {
			s.logger.Debug("radius health failure", "radius", peer.status.CommittedDepth, "peer_address", peer.addr, "bin", peer.bin)
		} else if peer.dur.Seconds() > pDur {
			s.logger.Debug("response duration below threshold", "duration", peer.dur, "peer_address", peer.addr, "bin", peer.bin)
		} else if peer.status.ConnectedPeers < pConns {
			s.logger.Debug("connections count below threshold", "connections", peer.status.ConnectedPeers, "peer_address", peer.addr, "bin", peer.bin)
		} else if peer.status.BatchCommitment != commitment {
			s.logger.Debug("batch commitment check failure", "commitment", peer.status.BatchCommitment, "peer_address", peer.addr, "bin", peer.bin)
		} else {
			healthy = true
		}

		s.topology.UpdatePeerHealth(peer.addr, healthy, peer.dur)
		if healthy {
			s.metrics.Healthy.Inc()
		} else {
			s.metrics.Unhealthy.Inc()
			bins[peer.bin]--
		}
	}

	selfHealth := true
	if nHoodRadius == networkRadius && s.reserve.CommittedDepth() != networkRadius {
		selfHealth = false
		s.logger.Warning("node is unhealthy due to storage radius discrepancy", "self_radius", s.reserve.CommittedDepth(), "network_radius", networkRadius)
	}

	s.isSelfHealthy.Store(selfHealth)

	s.publishRadius(networkRadius)
}

func (s *service) IsHealthy() bool {
	return s.isSelfHealthy.Load()
}

func (s *service) publishRadius(r uint8) {
	s.radiusSubsMtx.Lock()
	defer s.radiusSubsMtx.Unlock()
	for _, cb := range s.radiusC {
		select {
		case cb <- r:
		default:
		}
	}
}

func (s *service) SubscribeNetworkStorageRadius() (<-chan uint8, func()) {
	s.radiusSubsMtx.Lock()
	defer s.radiusSubsMtx.Unlock()

	c := make(chan uint8, 1)
	s.radiusC = append(s.radiusC, c)

	return c, func() {
		s.radiusSubsMtx.Lock()
		defer s.radiusSubsMtx.Unlock()
		for i, cc := range s.radiusC {
			if c == cc {
				s.radiusC = append(s.radiusC[:i], s.radiusC[i+1:]...)
				break
			}
		}
	}
}

// percentileDur finds the p percentile of response duration.
// Less is better.
func percentileDur(peers []peer, p float64) float64 {

	index := int(float64(len(peers)) * p)

	sort.Slice(peers, func(i, j int) bool {
		return peers[i].dur < peers[j].dur // ascending
	})

	return peers[index].dur.Seconds()
}

// percentileConns finds the p percentile of connection count.
// More is better.
func percentileConns(peers []peer, p float64) uint64 {

	index := int(float64(len(peers)) * p)

	sort.Slice(peers, func(i, j int) bool {
		return peers[i].status.ConnectedPeers > peers[j].status.ConnectedPeers // descending
	})

	return peers[index].status.ConnectedPeers
}

// radius finds the most common radius.
func (s *service) committedDepth(peers []peer) (uint8, uint8) {

	var networkDepth [swarm.MaxBins]int
	var nHoodDepth [swarm.MaxBins]int

	for _, peer := range peers {
		if peer.status.CommittedDepth < uint32(swarm.MaxBins) {
			if peer.neighbor {
				nHoodDepth[peer.status.CommittedDepth]++
			}
			networkDepth[peer.status.CommittedDepth]++
		}
	}

	networkD := maxIndex(networkDepth[:])
	hoodD := maxIndex(nHoodDepth[:])

	return uint8(networkD), uint8(hoodD)
}

// commitment finds the most common batch commitment.
func commitment(peers []peer) uint64 {

	commitments := make(map[uint64]int)

	for _, peer := range peers {
		commitments[peer.status.BatchCommitment]++
	}

	var (
		maxCount             = 0
		maxCommitment uint64 = 0
	)

	for commitment, count := range commitments {
		if count > maxCount {
			maxCommitment = commitment
			maxCount = count
		}
	}

	return maxCommitment
}

func maxIndex(n []int) int {

	maxValue := 0
	index := 0
	for i, c := range n {
		if c > maxValue {
			maxValue = c
			index = i
		}
	}

	return index
}
