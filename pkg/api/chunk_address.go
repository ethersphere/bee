// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

func (s *Service) hasChunkHandler(w http.ResponseWriter, r *http.Request) {
	str := mux.Vars(r)["address"]
	addr, err := swarm.ParseHexAddress(str)
	if err != nil {
		s.logger.Debug("has chunk: parse chunk address string failed", "string", str, "error", err)
		jsonhttp.BadRequest(w, "bad address")
		return
	}

	has, err := s.storer.Has(r.Context(), addr)
	if err != nil {
		s.logger.Debug("has chunk: has chunk failed", "chunk_address", addr, "error", err)
		jsonhttp.BadRequest(w, err)
		return
	}

	if !has {
		jsonhttp.NotFound(w, nil)
		return
	}
	jsonhttp.OK(w, nil)
}

func (s *Service) removeChunk(w http.ResponseWriter, r *http.Request) {
	str := mux.Vars(r)["address"]
	addr, err := swarm.ParseHexAddress(str)
	if err != nil {
		s.logger.Debug("remove chunk: parse chunk address string failed", "string", str, "error", err)
		jsonhttp.BadRequest(w, "bad address")
		return
	}

	has, err := s.storer.Has(r.Context(), addr)
	if err != nil {
		s.logger.Debug("remove chunk: has chunk failed", "chunk_address", addr, "error", err)
		jsonhttp.BadRequest(w, err)
		return
	}

	if !has {
		jsonhttp.OK(w, nil)
		return
	}

	err = s.storer.Set(r.Context(), storage.ModeSetRemove, addr)
	if err != nil {
		s.logger.Debug("remove chunk: remove chunk failed", "chunk_address", addr, "error", err)
		jsonhttp.InternalServerError(w, err)
		return
	}
	jsonhttp.OK(w, nil)
}
