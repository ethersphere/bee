// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/traversal"
	"github.com/gorilla/mux"
)

// pinBzz is used to pin an already uploaded content.
func (s *server) pinBzz(w http.ResponseWriter, r *http.Request) {
	addr, err := swarm.ParseHexAddress(mux.Vars(r)["address"])
	if err != nil {
		s.logger.Debugf("pin bzz: parse address: %v", err)
		s.logger.Error("pin bzz: parse address")
		jsonhttp.BadRequest(w, "bad address")
		return
	}

	has, err := s.storer.Has(r.Context(), addr)
	if err != nil {
		s.logger.Debugf("pin bzz: localstore has: %v", err)
		s.logger.Error("pin bzz: store")
		jsonhttp.InternalServerError(w, err)
		return
	}

	if !has {
		_, err := s.storer.Get(r.Context(), storage.ModeGetRequest, addr)
		if err != nil {
			s.logger.Debugf("pin chunk: netstore get: %v", err)
			s.logger.Error("pin chunk: netstore")

			jsonhttp.NotFound(w, nil)
			return
		}
	}

	ctx := r.Context()

	chunkAddressFn := s.pinChunkAddressFn(ctx, addr)

	err = s.traversal.TraverseManifestAddresses(ctx, addr, chunkAddressFn)
	if err != nil {
		s.logger.Debugf("pin bzz: traverse chunks: %v, addr %s", err, addr)

		if errors.Is(err, traversal.ErrInvalidType) {
			s.logger.Error("pin bzz: invalid type")
			jsonhttp.BadRequest(w, "invalid type")
			return
		}

		s.logger.Error("pin bzz: cannot pin")
		jsonhttp.InternalServerError(w, "cannot pin")
		return
	}

	jsonhttp.OK(w, nil)
}

// unpinBzz removes pinning from content.
func (s *server) unpinBzz(w http.ResponseWriter, r *http.Request) {
	addr, err := swarm.ParseHexAddress(mux.Vars(r)["address"])
	if err != nil {
		s.logger.Debugf("pin bzz: parse address: %v", err)
		s.logger.Error("pin bzz: parse address")
		jsonhttp.BadRequest(w, "bad address")
		return
	}

	has, err := s.storer.Has(r.Context(), addr)
	if err != nil {
		s.logger.Debugf("pin bzz: localstore has: %v", err)
		s.logger.Error("pin bzz: store")
		jsonhttp.InternalServerError(w, err)
		return
	}

	if !has {
		jsonhttp.NotFound(w, nil)
		return
	}

	ctx := r.Context()

	chunkAddressFn := s.unpinChunkAddressFn(ctx, addr)

	err = s.traversal.TraverseManifestAddresses(ctx, addr, chunkAddressFn)
	if err != nil {
		s.logger.Debugf("pin bzz: traverse chunks: %v, addr %s", err, addr)

		if errors.Is(err, traversal.ErrInvalidType) {
			s.logger.Error("pin bzz: invalid type")
			jsonhttp.BadRequest(w, "invalid type")
			return
		}

		s.logger.Error("pin bzz: cannot unpin")
		jsonhttp.InternalServerError(w, "cannot unpin")
		return
	}

	jsonhttp.OK(w, nil)
}
