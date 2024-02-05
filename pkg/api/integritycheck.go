// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package api

import (
	"encoding/json"
	"net/http"

	storer "github.com/ethersphere/bee/pkg/storer"
	"github.com/ethersphere/bee/pkg/swarm"
)

type PinIntegrityResponse struct {
	Ref     swarm.Address `json:"ref"`
	Total   int           `json:"total"`
	Missing int           `json:"missing"`
	Invalid int           `json:"invalid"`
}

func (s *Service) pinIntegrityHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_pin_integrity").Build()

	querie := struct {
		Ref swarm.Address `map:"ref"`
	}{}

	if response := s.mapStructure(r.URL.Query(), &querie); response != nil {
		response("invalid query params", logger, w)
		return
	}

	out := make(chan storer.PinStat)
	go s.pinIntegrity.Check(logger, querie.Ref.String(), out)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	enc := json.NewEncoder(w)

	for v := range out {
		resp := PinIntegrityResponse{
			Ref:     v.Ref,
			Total:   v.Total,
			Missing: v.Missing,
			Invalid: v.Invalid,
		}
		if err := enc.Encode(resp); err != nil {
			break
		}
		flusher.Flush()
	}
}
