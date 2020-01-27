// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/multiformats/go-multiaddr"
)

type peerConnectResponse struct {
	Address string
}

func (s *server) peerConnectHandler(w http.ResponseWriter, r *http.Request) {
	addr, err := multiaddr.NewMultiaddr("/" + mux.Vars(r)["multi-address"])
	if err != nil {
		panic(err)
	}

	address, err := s.P2P.Connect(r.Context(), addr)
	if err != nil {
		panic(err)
	}

	if err := json.NewEncoder(w).Encode(peerConnectResponse{
		Address: address,
	}); err != nil {
		panic(err)
	}
}
