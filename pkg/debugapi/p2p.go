// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/multiformats/go-multiaddr"
)

type addressesResponse struct {
	Addresses []multiaddr.Multiaddr `json:"addresses"`
}

func (s *server) addressesHandler(w http.ResponseWriter, r *http.Request) {
	addresses, err := s.P2P.Addresses()
	if err != nil {
		s.Logger.Debugf("debug api: p2p addresses: %v", err)
		jsonhttp.InternalServerError(w, err)
		return
	}
	jsonhttp.OK(w, addressesResponse{
		Addresses: addresses,
	})
}
