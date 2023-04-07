// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"sort"

	"github.com/ethersphere/bee/pkg/jsonhttp"
)

type statusLocalSnapshotResponse struct {
	ReserveSize   uint64  `json:"reserveSize"`
	PullsyncRate  float64 `json:"pullsyncRate"`
	StorageRadius uint8   `json:"storageRadius"`
}

type statusConnectedPeersSnapshotResponse struct {
	Peer             string  `json:"peer"`
	Proximity        uint8   `json:"proximity"`
	ReserveSize      uint64  `json:"reserveSize"`
	PullsyncRate     float64 `json:"pullsyncRate"`
	StorageRadius    uint8   `json:"storageRadius"`
	ConnectedPeers   uint64  `json:"connectedPeers"`
	NeighborhoodSize uint64  `json:"neighborhoodSize"`
	RequestFailed    bool    `json:"requestFailed,omitempty"`
}

type statusConnectedPeersResponse struct {
	Snapshots []statusConnectedPeersSnapshotResponse `json:"snapshots"`
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
func (s *Service) statusGetHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_status").Build()

	if s.beeMode == DevMode {
		logger.Warning("status endpoint is disabled in dev mode")
		jsonhttp.BadRequest(w, errUnsupportedDevNodeOperation)
		return
	}

	ss := s.statusService.LocalSnapshot()
	jsonhttp.OK(w, statusLocalSnapshotResponse{
		ReserveSize:   ss.ReserveSize,
		PullsyncRate:  ss.PullsyncRate,
		StorageRadius: ss.StorageRadius,
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

	sss, err := s.statusService.ConnectedPeersSnapshot(r.Context())
	if err != nil {
		logger.Debug("status snapshot", "error", err)
		logger.Error(nil, "status snapshot")
		jsonhttp.InternalServerError(w, err)
		return
	}

	sort.Slice(sss, func(i, j int) bool {
		return sss[i].Proximity < sss[j].Proximity
	})
	snapshots := make([]statusConnectedPeersSnapshotResponse, 0, len(sss))
	for _, ss := range sss {
		snapshots = append(snapshots, statusConnectedPeersSnapshotResponse{
			Peer:             ss.Peer.String(),
			Proximity:        ss.Proximity,
			ReserveSize:      ss.ReserveSize,
			PullsyncRate:     ss.PullsyncRate,
			StorageRadius:    ss.StorageRadius,
			ConnectedPeers:   ss.ConnectedPeers,
			NeighborhoodSize: ss.NeighborhoodSize,
			RequestFailed:    ss.RequestFailed,
		})
	}
	jsonhttp.OK(w, statusConnectedPeersResponse{Snapshots: snapshots})
}
