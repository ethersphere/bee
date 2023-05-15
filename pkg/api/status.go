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

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
)

type statusSnapshotResponse struct {
	Peer             string  `json:"peer"`
	Proximity        uint8   `json:"proximity"`
	BeeMode          string  `json:"beeMode"`
	ReserveSize      uint64  `json:"reserveSize"`
	PullsyncRate     float64 `json:"pullsyncRate"`
	StorageRadius    uint8   `json:"storageRadius"`
	ConnectedPeers   uint64  `json:"connectedPeers"`
	NeighborhoodSize uint64  `json:"neighborhoodSize"`
	RequestFailed    bool    `json:"requestFailed,omitempty"`
	BatchCommitment  uint64  `json:"batchCommitment"`
	IsReachable      bool    `json:"isReachable"`
}

type statusResponse struct {
	Snapshots []statusSnapshotResponse `json:"snapshots"`
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

	if s.beeMode == DevMode {
		logger.Warning("status endpoint is disabled in dev mode")
		jsonhttp.BadRequest(w, errUnsupportedDevNodeOperation)
		return
	}

	ss, err := s.statusService.LocalSnapshot()
	if err != nil {
		logger.Debug("status snapshot", "error", err)
		logger.Error(nil, "status snapshot")
		jsonhttp.InternalServerError(w, err)
		return
	}

	jsonhttp.OK(w, statusSnapshotResponse{
		Peer:             s.overlay.String(),
		BeeMode:          ss.BeeMode,
		ReserveSize:      ss.ReserveSize,
		PullsyncRate:     ss.PullsyncRate,
		StorageRadius:    uint8(ss.StorageRadius),
		ConnectedPeers:   ss.ConnectedPeers,
		NeighborhoodSize: ss.NeighborhoodSize,
		BatchCommitment:  ss.BatchCommitment,
		IsReachable:      ss.IsReachable,
	})
}

// statusGetPeersHandler returns the status of currently connected peers.
func (s *Service) statusGetPeersHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_status_peers").Build()

	if s.beeMode == DevMode {
		logger.Warning("status endpoint is disabled in dev mode")
		jsonhttp.BadRequest(w, errUnsupportedDevNodeOperation)
		return
	}

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
				Peer:      address.String(),
				Proximity: po,
			}

			ss, err := s.statusService.PeerSnapshot(ctx, address)
			if err != nil {
				logger.Debug("unable to get status snapshot for peer", "peer_address", address, "error", err)
				snapshot.RequestFailed = true
			} else {
				snapshot.BeeMode = ss.BeeMode
				snapshot.ReserveSize = ss.ReserveSize
				snapshot.PullsyncRate = ss.PullsyncRate
				snapshot.StorageRadius = uint8(ss.StorageRadius)
				snapshot.ConnectedPeers = ss.ConnectedPeers
				snapshot.NeighborhoodSize = ss.NeighborhoodSize
				snapshot.BatchCommitment = ss.BatchCommitment
				snapshot.IsReachable = ss.IsReachable
			}

			mu.Lock()
			snapshots = append(snapshots, snapshot)
			mu.Unlock()
		}()

		return false, false, nil
	}

	err := s.topologyDriver.EachConnectedPeer(
		peerFunc,
		topology.Filter{},
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
