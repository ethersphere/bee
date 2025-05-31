// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package salud monitors the connected peers, calculates certain thresholds, and marks peers as unhealthy that
// fall short of the thresholds to maintain network salud (health).
package salud

import (
	"context"
	"sort"
	"time"

	"github.com/ethersphere/bee/v2/pkg/stabilization"
	"github.com/ethersphere/bee/v2/pkg/status"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
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

func (s *service) worker(startupStabilizer stabilization.Subscriber, mode string, minPeersPerbin int, durPercentile float64, connsPercentile float64) {
	defer s.wg.Done()

	sub, unsubscribe := startupStabilizer.Subscribe()
	defer unsubscribe()

	select {
	case <-s.quit:
		return
	case <-sub:
		s.logger.Debug("node warmup check completed")
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
