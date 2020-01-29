// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

type pingpongResponse struct {
	RTT time.Duration `json:"rtt"`
}

func (s *server) pingpongHandler(w http.ResponseWriter, r *http.Request) {
	peerID := mux.Vars(r)["peer-id"]
	ctx := r.Context()

	address, err := swarm.ParseHexAddress(peerID)
	if err != nil {
		s.Logger.Debugf("pingpong: ping %s: %v", peerID, err)
		jsonhttp.BadRequest(w, "invalid peer address")
	}

	rtt, err := s.Pingpong.Ping(ctx, address, "hey", "there", ",", "how are", "you", "?")
	if err != nil {
		s.Logger.Debugf("pingpong: ping %s: %v", peerID, err)
		if errors.Is(err, p2p.ErrPeerNotFound) {
			jsonhttp.NotFound(w, "peer not found")
			return
		}

		s.Logger.Errorf("pingpong failed to peer %s", peerID)
		jsonhttp.InternalServerError(w, nil)
		return
	}
	s.metrics.PingRequestCount.Inc()

	s.Logger.Infof("pingpong succeeded to peer %s", peerID)
	jsonhttp.OK(w, pingpongResponse{
		RTT: rtt,
	})
}
