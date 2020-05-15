// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"encoding/json"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
)

type topologyResponse struct {
	Topology string `json:"topology"` //TODO: this is not so great since it makes the actual kademlia state to be an escaped string
}

func (s *server) topologyHandler(w http.ResponseWriter, r *http.Request) {
	ms, ok := s.TopologyDriver.(json.Marshaler)
	if !ok {
		s.Logger.Error("topology driver cast to json marshaler error")
		jsonhttp.InternalServerError(w, "topology json marshal interface error")
		return
	}

	bytes, err := ms.MarshalJSON()
	if err != nil {
		s.Logger.Errorf("topology marshal to json: %v", err)
		jsonhttp.InternalServerError(w, err)
		return
	}
	jsonhttp.OK(w, topologyResponse{Topology: string(bytes)})
}
