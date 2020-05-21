// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"context"
	"net/http"

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
		s.Logger.Debugf("debug api: pin chunk: parse chunk address: %v", err)
		jsonhttp.BadRequest(w, "bad address")
		return
	}

	has, err := s.Storer.Has(r.Context(), addr)
	if err != nil {
		s.Logger.Debugf("debug api: pin chunk: localstore has: %v", err)
		jsonhttp.BadRequest(w, err)
		return
	}

	if !has {
		jsonhttp.NotFound(w, nil)
		return
	}

	err = s.Storer.Set(r.Context(), storage.ModeSetPin, addr)
	if err != nil {
		s.Logger.Debugf("debug-api: pin chunk: pinning error: %v, addr %s", err, addr)
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
		s.Logger.Debugf("debug api: pin chunk: parse chunk ddress: %v", err)
		jsonhttp.BadRequest(w, "bad address")
		return
	}

	has, err := s.Storer.Has(r.Context(), addr)
	if err != nil {
		s.Logger.Debugf("debug api: pin chunk: localstore has: %v", err)
		jsonhttp.BadRequest(w, err)
		return
	}

	if !has {
		jsonhttp.NotFound(w, nil)
		return
	}

	_, err = s.Storer.PinInfo(addr)
	if err != nil {
		s.Logger.Debugf("debug api: pin chunk: not pinned: %v", err)
		jsonhttp.BadRequest(w, "chunk is not yet pinned")
		return
	}

	err = s.Storer.Set(r.Context(), storage.ModeSetUnpin, addr)
	if err != nil {
		s.Logger.Debugf("debug-api: pin chunk: unpinning error: %v, addr %s", err, addr)
		jsonhttp.InternalServerError(w, "cannot unpin chunk")
		return
	}
	jsonhttp.OK(w, nil)
}

// listPinnedChunks lists all the chunk address and pin counters that are currently pinned.
func (s *server) listPinnedChunks(w http.ResponseWriter, r *http.Request) {
	pinnedChunks, err := s.Storer.PinnedChunks(context.Background(), swarm.NewAddress(nil))
	if err != nil {
		s.Logger.Debugf("debug-api: pin chunk: listing pinned chunks error: %v", err)
		jsonhttp.InternalServerError(w, err)
		return
	}
	jsonhttp.OK(w, pinnedChunks)
}
