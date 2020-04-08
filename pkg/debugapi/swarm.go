// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
)

type overlayAddressResponse struct {
	Address swarm.Address `json:"address"`
}

func (s *server) overlayAddressHandler(w http.ResponseWriter, r *http.Request) {
	jsonhttp.OK(w, overlayAddressResponse{
		Address: s.Overlay,
	})
}
