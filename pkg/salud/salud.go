// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package salud

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/status"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
)

// TODO:
// logs
// metrics

const (
	DefaultWakeup       = time.Minute
	DefaultBlocklistDur = time.Minute * 5
)

type service struct {
	wg   sync.WaitGroup
	quit chan struct{}

	topology topology.Driver
	status   status.Service
	p2p      p2p.Blocklister
}

func New(warmup time.Duration) *service {

	s := &service{
		quit: make(chan struct{}),
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

// salud acquires the status snapshot of every peer and computes an avg response duration
// and the most common storage radius and based on these values, it blocklist peers that fall beyond
// some allowed threshold.
func (s *service) salud() {

	type peer struct {
		status *status.Snapshot
		dur    float64
		addr   swarm.Address
	}

	var (
		mtx sync.Mutex
		wg  sync.WaitGroup

		totaldur float64
		radiuses [swarm.MaxBins]int
		peers    []peer
	)

	_ = s.topology.EachConnectedPeer(func(addr swarm.Address, _ uint8) (stop bool, jumpToNext bool, err error) {
		wg.Add(1)
		go func() {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()

			start := time.Now()

			snapshot, err := s.status.PeerSnapshot(ctx, addr)
			if err != nil {
				// log and blocklist
				return
			}

			dur := time.Since(start).Seconds()

			mtx.Lock()
			totaldur += dur
			radiuses[snapshot.StorageRadius]++
			peers = append(peers, peer{status: snapshot, dur: dur, addr: addr})
			mtx.Unlock()
		}()
		return false, false, nil
	}, topology.Filter{Reachable: true})

	wg.Wait()

	maxValue := 0
	radius := 0
	for i, c := range radiuses {
		if c > maxValue {
			maxValue = c
			radius = i
		}
	}

	avgDur := totaldur / float64(len(peers))

	// log average dur and radius

	for _, peer := range peers {

		// radius check
		if radius > 0 && peer.status.StorageRadius < uint32(radius-1) {
			s.p2p.Blocklist(peer.addr, DefaultBlocklistDur, fmt.Sprintf("salud radius failure, radius %d", peer.status.StorageRadius))
			// log and blocklist
		}

		// duration check
		if peer.dur > avgDur*2 {
			s.p2p.Blocklist(peer.addr, DefaultBlocklistDur, fmt.Sprintf("salud radius failure, duration %0.1f", peer.dur))
			// log and blocklist
		}
	}
}
