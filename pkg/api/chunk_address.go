// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/mux"
)

func (s *Service) hasChunkHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_chunk").Build()

	paths := struct {
		Address swarm.Address `map:"address" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	address := paths.Address
	if v := getAddressFromContext(r.Context()); !v.Equal(swarm.ZeroAddress) {
		address = v
	}

	has, err := s.storer.ChunkStore().Has(r.Context(), address)
	if err != nil {
		logger.Debug("has chunk failed", "chunk_address", address, "error", err)
		jsonhttp.BadRequest(w, err)
		return
	}

	if !has {
		jsonhttp.NotFound(w, nil)
		return
	}
	jsonhttp.OK(w, nil)
}
