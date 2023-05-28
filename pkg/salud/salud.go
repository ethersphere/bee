// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package salud monitors the connected peers, calculates certain thresholds, and marks peers as unhealthy that
// fall short of the thresholds to maintain network salud (health).
package salud

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/status"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"go.uber.org/atomic"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "salud"

const (
	wakeup                      = time.Minute
	requestTimeout              = time.Second * 10
	DefaultMinPeersPerBin       = 3
	maxReserveSizePercentageErr = 0.02
)

type topologyDriver interface {
	UpdatePeerHealth(peer swarm.Address, health bool)
	topology.PeerIterator
}

type peerStatus interface {
	PeerSnapshot(ctx context.Context, peer swarm.Address) (*status.Snapshot, error)
}

type reserve interface {
	ReserveSize() uint64
}

type service struct {
	wg            sync.WaitGroup
	quit          chan struct{}
	logger        log.Logger
	topology      topologyDriver
	status        peerStatus
	metrics       metrics
	isSelfHealthy *atomic.Bool
	rad           postage.Radius
	reserve       reserve
}

func New(status peerStatus, topology topologyDriver, rad postage.Radius, reserve reserve, logger log.Logger, warmup time.Duration, mode string, minPeersPerbin int) *service {

	metrics := newMetrics()

	s := &service{
		quit:          make(chan struct{}),
		logger:        logger.WithName(loggerName).Register(),
		status:        status,
		topology:      topology,
		metrics:       metrics,
		isSelfHealthy: atomic.NewBool(true),
		rad:           rad,
		reserve:       reserve,
	}

	s.wg.Add(1)
	go s.worker(warmup, mode, minPeersPerbin)

	return s

}

func (s *service) worker(warmup time.Duration, mode string, minPeersPerbin int) {
	defer s.wg.Done()

	select {
	case <-s.quit:
		return
	case <-time.After(warmup):
	}

	for {

		s.salud(mode, minPeersPerbin)

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
	status *status.Snapshot
	dur    float64
	addr   swarm.Address
	bin    uint8
}

// salud acquires the status snapshot of every peer and computes an nth percentile of response duration and connected
// per count, the most common storage radius, and the batch commitment, and based on these values, marks peers as unhealhy that fall beyond
// the allowed thresholds.
func (s *service) salud(mode string, minPeersPerbin int) {

	var (
		mtx       sync.Mutex
		wg        sync.WaitGroup
		totaldur  float64
		peers     []peer
		neighbors int
		bins      [swarm.MaxBins]int
	)

	_ = s.topology.EachConnectedPeer(func(addr swarm.Address, bin uint8) (stop bool, jumpToNext bool, err error) {
		wg.Add(1)
		go func() {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
			defer cancel()

			start := time.Now()

			snapshot, err := s.status.PeerSnapshot(ctx, addr)
			if err != nil {
				s.topology.UpdatePeerHealth(addr, false)
				return
			}

			if snapshot.BeeMode != mode {
				return
			}

			dur := time.Since(start).Seconds()

			mtx.Lock()
			if s.rad.IsWithinStorageRadius(addr) {
				neighbors++
			}
			bins[bin]++
			totaldur += dur
			peers = append(peers, peer{snapshot, dur, addr, bin})
			mtx.Unlock()
		}()
		return false, false, nil
	}, topology.Filter{})

	wg.Wait()

	if len(peers) == 0 {
		return
	}

	percentile := 0.8
	networkRadius, nHoodRadius := s.radius(peers)
	avgDur := totaldur / float64(len(peers))
	pDur := percentileDur(peers, percentile)
	pConns := percentileConns(peers, percentile)
	commitment := commitment(peers)
	reserveSize := reserveSize(peers, networkRadius, (1-percentile)/2)

	s.metrics.AvgDur.Set(avgDur)
	s.metrics.PDur.Set(pDur)
	s.metrics.PConns.Set(float64(pConns))
	s.metrics.NetworkRadius.Set(float64(networkRadius))
	s.metrics.NeighborhoodRadius.Set(float64(nHoodRadius))
	s.metrics.Commitment.Set(float64(commitment))
	s.metrics.ReserveSize.Set(float64(reserveSize))

	s.logger.Debug("computed", "average", avgDur, "percentile", percentile, "pDur", pDur, "pConns", pConns, "network_radius", networkRadius, "neighborhood_radius", nHoodRadius, "batch_commitment", commitment, "reserve_size", reserveSize)

	for _, peer := range peers {

		var healthy bool

		// every bin should have at least some peers, healthy or not
		if bins[peer.bin] <= minPeersPerbin {
			s.topology.UpdatePeerHealth(peer.addr, true)
			continue
		}

		if networkRadius > 0 && peer.status.StorageRadius < uint32(networkRadius-1) {
			s.logger.Debug("radius health failure", "radius", peer.status.StorageRadius, "peer_address", peer.addr)
		} else if peer.dur > pDur {
			s.logger.Debug("dur health failure", "dur", peer.dur, "peer_address", peer.addr)
		} else if peer.status.ConnectedPeers < pConns {
			s.logger.Debug("connections health failure", "connections", peer.status.ConnectedPeers, "peer_address", peer.addr)
		} else if peer.status.BatchCommitment != commitment {
			s.logger.Debug("batch commitment health failure", "commitment", peer.status.BatchCommitment, "peer_address", peer.addr)
		} else {
			healthy = true
		}

		s.topology.UpdatePeerHealth(peer.addr, healthy)
		if healthy {
			s.metrics.Healthy.Inc()
		} else {
			s.metrics.Unhealthy.Inc()
			bins[peer.bin]--
		}
	}

	selfHealth := true
	if s.rad.StorageRadius() != networkRadius {
		selfHealth = false
		s.logger.Warning("node is unhealthy due to storage radius discrepency", "self_radius", s.rad.StorageRadius(), "network_radius", networkRadius)
	} else if reserveSize > 0 && percentageErr(float64(s.reserve.ReserveSize()), float64(reserveSize)) > maxReserveSizePercentageErr {
		s.logger.Warning("node is unhealthy due to reserve size discrepency", "self_size", s.reserve.ReserveSize(), "network_size", reserveSize)
		selfHealth = false
	}

	s.isSelfHealthy.Store(selfHealth)
}

func (s *service) IsHealthy() bool {
	return s.isSelfHealthy.Load()
}

// percentileDur finds the p percentile of response duration.
// Less is better.
func percentileDur(peers []peer, p float64) float64 {

	index := int(float64(len(peers)) * p)

	sort.Slice(peers, func(i, j int) bool {
		return peers[i].dur < peers[j].dur // ascending
	})

	return peers[index].dur
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
func (s *service) radius(peers []peer) (uint8, uint8) {

	var networkRadius [swarm.MaxBins]int
	var nHoodRadius [swarm.MaxBins]int

	for _, peer := range peers {
		if peer.status.StorageRadius < uint32(swarm.MaxBins) {
			if s.rad.IsWithinStorageRadius(peer.addr) {
				nHoodRadius[peer.status.StorageRadius]++
			}
			networkRadius[peer.status.StorageRadius]++
		}
	}

	networkR := maxIndex(networkRadius[:])
	hoodR := maxIndex(nHoodRadius[:])

	return uint8(networkR), uint8(hoodR)
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

// reserveSize returns the avg reserveSize, trimmed by p percent
func reserveSize(peers []peer, radius uint8, p float64) uint64 {

	startIndex := int(float64(len(peers)) * p)
	endIndex := int(float64(len(peers)) * (1 - p))

	sort.Slice(peers, func(i, j int) bool {
		return peers[i].status.ReserveSize < peers[j].status.ReserveSize // ascendings
	})

	var avg uint64
	var count uint64

	for _, peer := range peers[startIndex:endIndex] {
		if peer.status.StorageRadius == uint32(radius) {
			avg += peer.status.ReserveSize
			count++
		}
	}

	if count > 0 {
		return avg / count
	}

	return 0

}

func percentageErr(x, y float64) float64 {
	return math.Abs((x - y) / y)
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
