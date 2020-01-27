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
	"github.com/gorilla/mux"
)

type pingpongResponse struct {
	RTT time.Duration `json:"rtt"`
}

func (s *server) pingpongHandler(w http.ResponseWriter, r *http.Request) {
	peerID := mux.Vars(r)["peer-id"]
	ctx := r.Context()

	rtt, err := s.Pingpong.Ping(ctx, peerID, "hey", "there", ",", "how are", "you", "?")
	if err != nil {
		if errors.Is(err, p2p.ErrPeerNotFound) {
			s.Logger.Debugf("pingpong: ping %s: %v", peerID, err)
			jsonhttp.NotFound(w, "peer not found")
			return
		}
		s.Logger.Errorf("pingpong: ping %s: %v", peerID, err)
		jsonhttp.InternalServerError(w, err)
		return
	}
	s.metrics.PingRequestCount.Inc()

	jsonhttp.OK(w, pingpongResponse{
		RTT: rtt,
	})
}
