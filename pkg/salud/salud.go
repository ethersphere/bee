// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package salud monitors the storage radius and request reponse duration of peers
// and blocklists peers to maintain network salud (health).
package salud

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/status"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "salud"

const (
	DefaultWakeup         = time.Minute
	DefaultBlocklistDur   = time.Minute * 5
	DefaultRequestTimeout = time.Second * 10
)

type service struct {
	wg          sync.WaitGroup
	quit        chan struct{}
	logger      log.Logger
	topology    topology.Driver
	status      *status.Service
	metrics     metrics
	blocklister func() p2p.Blocklister
}

func New(status *status.Service, topology topology.Driver, blocklister p2p.Blocklister, logger log.Logger, warmup time.Duration) *service {

	metrics := newMetrics()

	s := &service{
		quit:     make(chan struct{}),
		logger:   logger.WithName(loggerName).Register(),
		status:   status,
		topology: topology,
		metrics:  metrics,
		blocklister: func() p2p.Blocklister {
			metrics.Blocklisted.Inc()
			return blocklister
		},
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
		select {
		case <-s.quit:
			return
		case <-time.After(DefaultWakeup):
			s.salud()
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
		mtx sync.Mutex
		wg  sync.WaitGroup

		totaldur float64
		peers    []peer
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
				s.blocklister().Blocklist(addr, DefaultBlocklistDur, "salud status snapshop failure")
				return
			}

			if snapshot.BeeMode != "full" {
				return
			}

			dur := time.Since(start).Seconds()

			// s.logger.Debug("status", "radius", snapshot.StorageRadius, "dur", dur, "mode", snapshot.BeeMode, "peer", addr)

			mtx.Lock()
			totaldur += dur
			peers = append(peers, peer{status: snapshot, dur: dur, addr: addr})
			mtx.Unlock()
		}()
		return false, false, nil
	}, topology.Filter{})

	wg.Wait()

	radius := radius(peers)
	avgDur := totaldur / float64(len(peers))
	pDur := percantile(peers, .99)

	s.metrics.Dur.Set(avgDur)
	s.metrics.Radius.Set(float64(radius))

	s.logger.Debug("computed", "average", avgDur, "p99", pDur, "radius", radius)

	for _, peer := range peers {

		// radius check
		if radius > 0 && peer.status.StorageRadius < uint32(radius-1) {
			s.blocklister().Blocklist(peer.addr, DefaultBlocklistDur, fmt.Sprintf("salud radius failure, radius %d", peer.status.StorageRadius))
		}

		// duration check
		if peer.dur > pDur {
			s.blocklister().Blocklist(peer.addr, DefaultBlocklistDur, fmt.Sprintf("salud duration exceeded, duration %0.1f", peer.dur))
		}
	}
}

func percantile(peers []peer, p float64) float64 {

	index := int(float64(len(peers)) * p)

	sort.Slice(peers, func(i, j int) bool {
		return peers[i].dur < peers[j].dur
	})

	return peers[index].dur
}

func radius(peers []peer) uint8 {

	var radiuses [swarm.MaxBins]int

	for _, peer := range peers {
		if peer.status.StorageRadius < uint32(swarm.MaxBins) {
			radiuses[peer.status.StorageRadius]++
		}
	}

	maxValue := 0
	radius := 0
	for i, c := range radiuses {
		if c > maxValue {
			maxValue = c
			radius = i
		}
	}

	return uint8(radius)
}
