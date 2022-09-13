// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"github.com/gorilla/mux"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

func (s *Service) hasChunkHandler(w http.ResponseWriter, r *http.Request) {
	path := struct {
		Address []byte `parse:"address,addressToString" name:"address" errMessage:"bad address"`
	}{}

	if err := s.parseAndValidate(r, &path); err != nil {
		s.logger.Debug("has chunk: parse chunk address string failed", "string", mux.Vars(r)["address"], "error", err)
		jsonhttp.BadRequest(w, err.Error())
		return
	}
	has, err := s.storer.Has(r.Context(), swarm.NewAddress(path.Address))
	if err != nil {
		s.logger.Debug("has chunk: has chunk failed", "chunk_address", swarm.NewAddress(path.Address), "error", err)
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
	path := struct {
		Address []byte `parse:"address,addressToString" name:"address" errMessage:"invalid address"`
	}{}

	if err := s.parseAndValidate(r, &path); err != nil {
		s.logger.Debug("remove chunk: parse chunk address string failed", "string", mux.Vars(r)["address"], "error", err)
		jsonhttp.BadRequest(w, err.Error())
		return
	}

	has, err := s.storer.Has(r.Context(), swarm.NewAddress(path.Address))
	if err != nil {
		s.logger.Debug("remove chunk: has chunk failed", "chunk_address", swarm.NewAddress(path.Address), "error", err)
		jsonhttp.BadRequest(w, err)
		return
	}

	if !has {
		jsonhttp.OK(w, nil)
		return
	}

	err = s.storer.Set(r.Context(), storage.ModeSetRemove, swarm.NewAddress(path.Address))
	if err != nil {
		s.logger.Debug("remove chunk: remove chunk failed", "chunk_address", swarm.NewAddress(path.Address), "error", err)
		jsonhttp.InternalServerError(w, err)
		return
	}
	jsonhttp.OK(w, nil)
}
