// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"net/http"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/mux"
)

type pingpongResponse struct {
	RTT string `json:"rtt"`
}

func (s *Service) pingpongHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("post_pinpong").Build()

	paths := struct {
		Address swarm.Address `map:"address" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	ctx := r.Context()
	span, logger, ctx := s.tracer.StartSpanFromContext(ctx, "pingpong-api", logger)
	defer span.Finish()

	rtt, err := s.pingpong.Ping(ctx, paths.Address, "ping")
	if err != nil {
		logger.Debug("pingpong: ping failed", "peer_address", paths.Address, "error", err)
		if errors.Is(err, p2p.ErrPeerNotFound) {
			jsonhttp.NotFound(w, "peer not found")
			return
		}

		logger.Error(nil, "pingpong: ping failed", "peer_address", paths.Address)
		jsonhttp.InternalServerError(w, "pingpong: ping failed")
		return
	}

	logger.Info("pingpong: ping succeeded", "peer_address", paths.Address)
	jsonhttp.OK(w, pingpongResponse{
		RTT: rtt.String(),
	})
}
