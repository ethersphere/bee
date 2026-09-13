// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
)

type statusSnapshotResponse struct {
	Overlay                 string  `json:"overlay"`
	Proximity               uint    `json:"proximity"`
	BeeMode                 string  `json:"beeMode"`
	ReserveSize             uint64  `json:"reserveSize"`
	ReserveSizeWithinRadius uint64  `json:"reserveSizeWithinRadius"`
	PullsyncRate            float64 `json:"pullsyncRate"`
	StorageRadius           uint8   `json:"storageRadius"`
	ConnectedPeers          uint64  `json:"connectedPeers"`
	NeighborhoodSize        uint64  `json:"neighborhoodSize"`
	RequestFailed           bool    `json:"requestFailed,omitempty"`
	BatchCommitment         uint64  `json:"batchCommitment"`
	IsReachable             bool    `json:"isReachable"`
	LastSyncedBlock         uint64  `json:"lastSyncedBlock"`
	CommittedDepth          uint8   `json:"committedDepth"`
	IsWarmingUp             bool    `json:"isWarmingUp"`
	BeeStatus               string  `json:"beeStatus,omitempty"`
}

type statusResponse struct {
	Snapshots []statusSnapshotResponse `json:"snapshots"`
}

type statusNeighborhoodResponse struct {
	Neighborhood            string `json:"neighborhood"`
	ReserveSizeWithinRadius int    `json:"reserveSizeWithinRadius"`
	Proximity               uint8  `json:"proximity"`
}

type neighborhoodsResponse struct {
	Neighborhoods []statusNeighborhoodResponse `json:"neighborhoods"`
}

// statusAccessHandler is a middleware that limits the number of simultaneous
// status requests.
func (s *Service) statusAccessHandler(h http.Handler) http.Handler {
	logger := s.logger.WithName("status_access").Build()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.statusSem.TryAcquire(1) {
			logger.Debug("simultaneous operations not supported")
			logger.Error(nil, "simultaneous operations not supported")
			jsonhttp.TooManyRequests(w, "simultaneous operations not supported")
			return
		}
		defer s.statusSem.Release(1)

		h.ServeHTTP(w, r)
	})
}

// statusGetHandler returns the current node status.
func (s *Service) statusGetHandler(w http.ResponseWriter, _ *http.Request) {
	logger := s.logger.WithName("get_status").Build()

	overlay := ""
	if s.overlay != nil {
		overlay = s.overlay.String()
	}

	resp := statusSnapshotResponse{
		Proximity:   256,
		Overlay:     overlay,
		BeeMode:     s.beeMode.String(),
		IsWarmingUp: s.isWarmingUp,
		BeeStatus:   s.BeeStatusString(),
	}

	if s.batchStore != nil {
		if commitment, err := s.batchStore.Commitment(); err == nil {
			resp.BatchCommitment = commitment
		}
		if cs := s.batchStore.GetChainState(); cs != nil {
			resp.LastSyncedBlock = cs.Block
		}
	}

	if s.statusService == nil {
		jsonhttp.OK(w, resp)
		return
	}

	ss, err := s.statusService.LocalSnapshot()
	if err != nil {
		logger.Debug("status snapshot", "error", err)
		logger.Error(nil, "status snapshot")
		jsonhttp.InternalServerError(w, err)
		return
	}

	resp.BeeMode = ss.BeeMode
	resp.ReserveSize = ss.ReserveSize
	resp.ReserveSizeWithinRadius = ss.ReserveSizeWithinRadius
	resp.PullsyncRate = ss.PullsyncRate
	resp.StorageRadius = uint8(ss.StorageRadius)
	resp.ConnectedPeers = ss.ConnectedPeers
	resp.NeighborhoodSize = ss.NeighborhoodSize
	resp.BatchCommitment = ss.BatchCommitment
	resp.IsReachable = ss.IsReachable
	resp.LastSyncedBlock = ss.LastSyncedBlock
	resp.CommittedDepth = uint8(ss.CommittedDepth)

	jsonhttp.OK(w, resp)
}

// statusGetPeersHandler returns the status of currently connected peers.
func (s *Service) statusGetPeersHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_status_peers").Build()

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex // mu protects snapshots.
		snapshots []statusSnapshotResponse
	)

	peerFunc := func(address swarm.Address, po uint8) (bool, bool, error) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)

		wg.Add(1)
		go func() {
			defer cancel()
			defer wg.Done()

			snapshot := statusSnapshotResponse{
				Overlay:   address.String(),
				Proximity: uint(po),
			}

			ss, err := s.statusService.PeerSnapshot(ctx, address)
			if err != nil {
				logger.Debug("unable to get status snapshot for peer", "peer_address", address, "error", err)
				snapshot.RequestFailed = true
			} else {
				snapshot.BeeMode = ss.BeeMode
				snapshot.ReserveSize = ss.ReserveSize
				snapshot.ReserveSizeWithinRadius = ss.ReserveSizeWithinRadius
				snapshot.PullsyncRate = ss.PullsyncRate
				snapshot.StorageRadius = uint8(ss.StorageRadius)
				snapshot.ConnectedPeers = ss.ConnectedPeers
				snapshot.NeighborhoodSize = ss.NeighborhoodSize
				snapshot.BatchCommitment = ss.BatchCommitment
				snapshot.IsReachable = ss.IsReachable
				snapshot.LastSyncedBlock = ss.LastSyncedBlock
				snapshot.CommittedDepth = uint8(ss.CommittedDepth)
			}

			mu.Lock()
			snapshots = append(snapshots, snapshot)
			mu.Unlock()
		}()

		return false, false, nil
	}

	err := s.topologyDriver.EachConnectedPeer(
		peerFunc,
		topology.Select{IncludeBootnodes: true},
	)
	if err != nil {
		logger.Debug("status snapshot", "error", err)
		logger.Error(nil, "status snapshot")
		jsonhttp.InternalServerError(w, err)
		return
	}

	wg.Wait()

	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Proximity < snapshots[j].Proximity
	})
	jsonhttp.OK(w, statusResponse{Snapshots: snapshots})
}

// statusGetHandler returns the current node status.
func (s *Service) statusGetNeighborhoods(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_status_neighborhoods").Build()

	neighborhoods := make([]statusNeighborhoodResponse, 0)

	nhoods, err := s.storer.NeighborhoodsStat(r.Context())
	if err != nil {
		logger.Debug("unable to get neighborhoods status", "error", err)
		logger.Error(nil, "unable to get neighborhoods status")
		jsonhttp.InternalServerError(w, "unable to get neighborhoods status")
		return
	}

	for _, n := range nhoods {
		neighborhoods = append(neighborhoods, statusNeighborhoodResponse{
			Neighborhood:            n.Neighborhood.String(),
			ReserveSizeWithinRadius: n.ReserveSizeWithinRadius,
			Proximity:               n.Proximity,
		})
	}

	jsonhttp.OK(w, neighborhoodsResponse{Neighborhoods: neighborhoods})
}
