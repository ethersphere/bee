//go:build !js
// +build !js

package salud

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/stabilization"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	"go.uber.org/atomic"
)

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
	startupStabilizer stabilization.Subscriber,
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
	go s.worker(startupStabilizer, mode, minPeersPerbin, durPercentile, connsPercentile)

	return s
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

	err := s.topology.EachConnectedPeer(func(addr swarm.Address, bin uint8) (stop bool, jumpToNext bool, err error) {
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
	if err != nil {
		s.logger.Error(err, "error iterating over connected peers", "mode", mode)
	}

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
