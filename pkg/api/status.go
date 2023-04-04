// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"sort"

	"github.com/ethersphere/bee/pkg/jsonhttp"
)

type statusSnapshotResponse struct {
	Peer          string  `json:"peer,omitempty"`
	Proximity     uint8   `json:"proximity"`
	ReserveSize   uint64  `json:"reserveSize"`
	PullsyncRate  float64 `json:"pullsyncRate"`
	StorageRadius uint8   `json:"storageRadius"`
	RequestFailed bool    `json:"requestFailed,omitempty"`
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
func (s *Service) statusGetHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_status").Build()

	if s.beeMode == DevMode {
		logger.Warning("status endpoint is disabled in dev mode")
		jsonhttp.BadRequest(w, errUnsupportedDevNodeOperation)
		return
	}

	queries := struct {
		ConnectedPeers bool `map:"connectedPeers"`
	}{}
	if response := s.mapStructure(r.URL.Query(), &queries); response != nil {
		response("invalid query params", logger, w)
		return
	}

	sss, err := s.statusService.Snapshots(r.Context(), queries.ConnectedPeers)
	if err != nil {
		logger.Debug("status snapshot", "error", err)
		logger.Error(nil, "status snapshot")
		jsonhttp.InternalServerError(w, err)
		return
	}

	sort.Slice(sss, func(i, j int) bool {
		return sss[i].Proximity < sss[j].Proximity
	})
	snapshots := make([]statusSnapshotResponse, 0, len(sss))
	for _, ss := range sss {
		snapshots = append(snapshots, statusSnapshotResponse{
			Peer:          ss.Peer.String(),
			Proximity:     ss.Proximity,
			ReserveSize:   ss.ReserveSize,
			PullsyncRate:  ss.PullsyncRate,
			StorageRadius: ss.StorageRadius,
			RequestFailed: ss.RequestFailed,
		})
	}
	jsonhttp.OK(w, statusResponse{Snapshots: snapshots})
}
