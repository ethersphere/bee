// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

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
		logger.Debugf("pingpong: parse peer address %s: %v", peerID, err)
		jsonhttp.BadRequest(w, "invalid peer address")
		return
	}

	rtt, err := s.pingpong.Ping(ctx, address, "hey", "there", ",", "how are", "you", "?")
	if err != nil {
		logger.Debugf("pingpong: ping %s: %v", peerID, err)
		if errors.Is(err, p2p.ErrPeerNotFound) {
			jsonhttp.NotFound(w, "peer not found")
			return
		}

		logger.Errorf("pingpong failed to peer %s", peerID)
		jsonhttp.InternalServerError(w, nil)
		return
	}

	logger.Infof("pingpong succeeded to peer %s", peerID)
	jsonhttp.OK(w, pingpongResponse{
		RTT: rtt.String(),
	})
}
