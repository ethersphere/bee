// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"github.com/gorilla/mux"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
)

type pingpongResponse struct {
	RTT string `json:"rtt"`
}

func (s *Service) pingpongHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span, logger, ctx := s.tracer.StartSpanFromContext(ctx, "pingpong-api", s.logger)
	defer span.Finish()

	path := struct {
		PeerID []byte `parse:"peer-id,addressToString" name:"peer address"`
	}{}
	err := s.parseAndValidate(mux.Vars(r), &path)
	if err != nil {
		s.logger.Debug("pingpong:: decode string failed", "struct", path, "error", err)
		s.logger.Error(nil, "pingpong: decode string failed")
		jsonhttp.BadRequest(w, err.Error())
		return
	}
	address := swarm.NewAddress(path.PeerID)
	rtt, err := s.pingpong.Ping(ctx, address, "ping")
	if err != nil {
		logger.Debug("pingpong: ping failed", "peer_address", address, "error", err)
		if errors.Is(err, p2p.ErrPeerNotFound) {
			jsonhttp.NotFound(w, "peer not found")
			return
		}

		logger.Error(nil, "pingpong: ping failed", "peer_address", address)
		jsonhttp.InternalServerError(w, "pingpong: ping failed")
		return
	}

	logger.Info("pingpong: ping succeeded", "peer_address", address)
	jsonhttp.OK(w, pingpongResponse{
		RTT: rtt.String(),
	})
}
