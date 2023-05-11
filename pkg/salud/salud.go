// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package salud monitors the storage radius and request response duration of peers
// and blocklists peers to maintain network salud (health).
package salud

import (
	"context"
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
	DefaultWakeup         = time.Minute
	DefaultRequestTimeout = time.Second * 10
)

type topologyDriver interface {
	UpdatePeerHealth(peer swarm.Address, health bool)
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
	rs            postage.Radius
}

func New(status peerStatus, topology topologyDriver, rs postage.Radius, logger log.Logger, warmup time.Duration) *service {

	metrics := newMetrics()

	s := &service{
		quit:          make(chan struct{}),
		logger:        logger.WithName(loggerName).Register(),
		status:        status,
		topology:      topology,
		metrics:       metrics,
		isSelfHealthy: atomic.NewBool(true),
		rs:            rs,
	}

	s.wg.Add(1)
	go s.worker(warmup)

	return s

}

func (s *service) worker(warmup time.Duration) {
	defer s.wg.Done()

	select {
	case <-s.quit:
		return
	case <-time.After(warmup):
	}

	for {

		s.salud()

		select {
		case <-s.quit:
			return
		case <-time.After(DefaultWakeup):
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
}

// salud acquires the status snapshot of every peer and computes an avg response duration
// and the most common storage radius and based on these values, it blocklist peers that fall beyond
// some allowed threshold.
func (s *service) salud() {

	var (
		mtx       sync.Mutex
		wg        sync.WaitGroup
		totaldur  float64
		peers     []peer
		neighbors int
	)

	_ = s.topology.EachConnectedPeer(func(addr swarm.Address, _ uint8) (stop bool, jumpToNext bool, err error) {
		wg.Add(1)
		go func() {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), DefaultRequestTimeout)
			defer cancel()

			start := time.Now()

			snapshot, err := s.status.PeerSnapshot(ctx, addr)
			if err != nil {
				s.topology.UpdatePeerHealth(addr, false)
				return
			}

			if snapshot.BeeMode != "full" {
				return
			}

			dur := time.Since(start).Seconds()

			mtx.Lock()
			if s.rs.IsWithinStorageRadius(addr) {
				neighbors++
			}
			totaldur += dur
			peers = append(peers, peer{status: snapshot, dur: dur, addr: addr})
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

	s.metrics.AvgDur.Set(avgDur)
	s.metrics.PDur.Set(pDur)
	s.metrics.PConns.Set(float64(pConns))
	s.metrics.NetworkRadius.Set(float64(networkRadius))
	s.metrics.NeighborhoodRadius.Set(float64(nHoodRadius))

	s.logger.Debug("computed", "average", avgDur, "percentile", percentile, "pDur", pDur, "pConns", pConns, "network_radius", networkRadius, "neighborhood_radius", nHoodRadius)

	for _, peer := range peers {

		var healthy bool

		if networkRadius > 0 && peer.status.StorageRadius < uint32(networkRadius-1) {
			s.logger.Debug("radius health failure", "radius", peer.status.StorageRadius, "peer_address", peer.addr)
		} else if peer.dur > pDur {
			s.logger.Debug("dur health failure", "dur", peer.dur, "peer_address", peer.addr)
		} else if peer.status.ConnectedPeers < pConns {
			s.logger.Debug("connections health failure", "connections", peer.status.ConnectedPeers, "peer_address", peer.addr)
		} else {
			healthy = true
		}

		s.topology.UpdatePeerHealth(peer.addr, healthy)
		if healthy {
			s.metrics.Healthy.Inc()
		} else {
			s.metrics.Unhealthy.Inc()
		}
	}

	if neighbors > 0 {
		s.isSelfHealthy.Store(s.rs.StorageRadius() == nHoodRadius)
	}
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
			if s.rs.IsWithinStorageRadius(peer.addr) {
				nHoodRadius[peer.status.StorageRadius]++
			}
			networkRadius[peer.status.StorageRadius]++
		}
	}

	networkR := highestCollisions(networkRadius[:])
	hoodR := highestCollisions(nHoodRadius[:])

	return uint8(networkR), uint8(hoodR)
}

func highestCollisions(n []int) int {

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
