// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

type pingpongResponse struct {
	RTT string `json:"rtt"`
}

func (s *Service) pingpongHandler(w http.ResponseWriter, r *http.Request) {
	peerID := mux.Vars(r)["peer-id"]
	ctx := r.Context()

	span, logger, ctx := s.tracer.StartSpanFromContext(ctx, "pingpong-api", s.logger)
	defer span.Finish()

	address, err := swarm.ParseHexAddress(peerID)
	if err != nil {
		logger.Debug("pingpong: parse peer address string failed", "string", peerID, "error", err)
		jsonhttp.BadRequest(w, "invalid peer address")
		return
	}

	rtt, err := s.pingpong.Ping(ctx, address, "ping")
	if err != nil {
		logger.Debug("pingpong: ping failed", "address", address, "error", err)
		if errors.Is(err, p2p.ErrPeerNotFound) {
			jsonhttp.NotFound(w, "peer not found")
			return
		}

		logger.Error(nil, "pingpong: ping failed", "address", address)
		jsonhttp.InternalServerError(w, "pingpong: ping failed")
		return
	}

	logger.Info("pingpong: ping succeeded", "address", address)
	jsonhttp.OK(w, pingpongResponse{
		RTT: rtt.String(),
	})
}
