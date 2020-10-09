// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

// pinChunk pin's the already created chunk given its address.
// it fails if the chunk is not present in the local store.
// It also increments a pin counter to keep track of how many pin requests
// are originating for this chunk.
func (s *server) pinChunk(w http.ResponseWriter, r *http.Request) {
	addr, err := swarm.ParseHexAddress(mux.Vars(r)["address"])
	if err != nil {
		s.Logger.Debugf("pin chunk: parse chunk address: %v", err)
		s.Logger.Error("pin chunk: parse address")
		jsonhttp.BadRequest(w, "bad address")
		return
	}

	has, err := s.Storer.Has(r.Context(), addr)
	if err != nil {
		s.Logger.Debugf("pin chunk: localstore has: %v", err)
		s.Logger.Error("pin chunk: store")
		jsonhttp.InternalServerError(w, err)
		return
	}

	if !has {
		jsonhttp.NotFound(w, nil)
		return
	}

	err = s.Storer.Set(r.Context(), storage.ModeSetPin, addr)
	if err != nil {
		s.Logger.Debugf("pin chunk: pinning error: %v, addr %s", err, addr)
		s.Logger.Error("pin chunk: cannot pin chunk")
		jsonhttp.InternalServerError(w, "cannot pin chunk")
		return
	}
	jsonhttp.OK(w, nil)
}

// unpinChunk unpin's an already pinned chunk. If the chunk is not present or the
// if the pin counter is zero, it raises error.
func (s *server) unpinChunk(w http.ResponseWriter, r *http.Request) {
	addr, err := swarm.ParseHexAddress(mux.Vars(r)["address"])
	if err != nil {
		s.Logger.Debugf("pin chunk: parse chunk address: %v", err)
		s.Logger.Error("pin chunk: parse address")
		jsonhttp.BadRequest(w, "bad address")
		return
	}

	has, err := s.Storer.Has(r.Context(), addr)
	if err != nil {
		s.Logger.Debugf("pin chunk: localstore has: %v", err)
		s.Logger.Error("pin chunk: store")
		jsonhttp.InternalServerError(w, err)
		return
	}

	if !has {
		jsonhttp.NotFound(w, nil)
		return
	}

	_, err = s.Storer.PinCounter(addr)
	if err != nil {
		s.Logger.Debugf("pin chunk: not pinned: %v", err)
		s.Logger.Error("pin chunk: pin counter")
		jsonhttp.BadRequest(w, "chunk is not yet pinned")
		return
	}

	err = s.Storer.Set(r.Context(), storage.ModeSetUnpin, addr)
	if err != nil {
		s.Logger.Debugf("pin chunk: unpinning error: %v, addr %s", err, addr)
		s.Logger.Error("pin chunk: unpin")
		jsonhttp.InternalServerError(w, "cannot unpin chunk")
		return
	}
	jsonhttp.OK(w, nil)
}

type pinnedChunk struct {
	Address    swarm.Address `json:"address"`
	PinCounter uint64        `json:"pinCounter"`
}

type listPinnedChunksResponse struct {
	Chunks []pinnedChunk `json:"chunks"`
}

// listPinnedChunks lists all the chunk address and pin counters that are currently pinned.
func (s *server) listPinnedChunks(w http.ResponseWriter, r *http.Request) {
	var (
		err           error
		offset, limit = 0, 100 // default offset is 0, default limit 100
	)

	if v := r.URL.Query().Get("offset"); v != "" {
		offset, err = strconv.Atoi(v)
		if err != nil {
			s.Logger.Debugf("list pins: parse offset: %v", err)
			s.Logger.Errorf("list pins: bad offset")
			jsonhttp.BadRequest(w, "bad offset")
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, err = strconv.Atoi(v)
		if err != nil {
			s.Logger.Debugf("list pins: parse limit: %v", err)
			s.Logger.Errorf("list pins: bad limit")
			jsonhttp.BadRequest(w, "bad limit")
		}
	}

	pinnedChunks, err := s.Storer.PinnedChunks(r.Context(), offset, limit)
	if err != nil {
		s.Logger.Debugf("list pins: list pinned: %v", err)
		s.Logger.Errorf("list pins: list pinned")
		jsonhttp.InternalServerError(w, err)
		return
	}

	chunks := make([]pinnedChunk, len(pinnedChunks))
	for i, c := range pinnedChunks {
		chunks[i] = pinnedChunk(*c)
	}

	jsonhttp.OK(w, listPinnedChunksResponse{
		Chunks: chunks,
	})
}

func (s *server) getPinnedChunk(w http.ResponseWriter, r *http.Request) {
	addr, err := swarm.ParseHexAddress(mux.Vars(r)["address"])
	if err != nil {
		s.Logger.Debugf("pin counter: parse chunk ddress: %v", err)
		s.Logger.Errorf("pin counter: parse address")
		jsonhttp.NotFound(w, nil)
		return
	}

	has, err := s.Storer.Has(r.Context(), addr)
	if err != nil {
		s.Logger.Debugf("pin counter: localstore has: %v", err)
		s.Logger.Errorf("pin counter: store")
		jsonhttp.NotFound(w, nil)
		return
	}

	if !has {
		jsonhttp.NotFound(w, nil)
		return
	}

	pinCounter, err := s.Storer.PinCounter(addr)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			jsonhttp.NotFound(w, nil)
			return
		}
		s.Logger.Debugf("pin counter: get pin counter: %v", err)
		s.Logger.Errorf("pin counter: get pin counter")
		jsonhttp.InternalServerError(w, err)
		return
	}
	jsonhttp.OK(w, pinnedChunk{
		Address:    addr,
		PinCounter: pinCounter,
	})
}
