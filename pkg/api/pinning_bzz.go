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

// pinBzzUploaded is used to pin an already uploaded content.
func (s *server) pinBzzUploaded(w http.ResponseWriter, r *http.Request) {
	addr, err := swarm.ParseHexAddress(mux.Vars(r)["address"])
	if err != nil {
		s.Logger.Debugf("pin bzz: parse address: %v", err)
		s.Logger.Error("pin bzz: parse address")
		jsonhttp.BadRequest(w, "bad address")
		return
	}

	has, err := s.Storer.Has(r.Context(), addr)
	if err != nil {
		s.Logger.Debugf("pin bzz: localstore has: %v", err)
		s.Logger.Error("pin bzz: store")
		jsonhttp.InternalServerError(w, err)
		return
	}

	if !has {
		jsonhttp.NotFound(w, nil)
		return
	}

	ctx := r.Context()

	chunkAddressFn := func(address swarm.Address) (stop bool) {
		err := s.Storer.Set(r.Context(), storage.ModeSetPin, address)
		if err != nil {
			s.Logger.Debugf("pin bzz: pinning error: %v, addr %s, for address: %s", err, addr, address)
			// stop pinning on first error
			return true
		}

		return false
	}

	err = s.Traversal.TraverseManifestAddresses(ctx, addr, chunkAddressFn)
	if err != nil {
		s.Logger.Debugf("pin bzz: traverse chunks: %v, addr %s", err, addr)

		if errors.Is(err, traversal.ErrInvalidType) {
			s.Logger.Error("pin bzz: invalid type")
			jsonhttp.BadRequest(w, "invalid type")
			return
		}

		s.Logger.Error("pin bzz: cannot pin")
		jsonhttp.InternalServerError(w, "cannot pin")
		return
	}

	jsonhttp.OK(w, nil)
}

// pinBzzRemovePinned removes pinning from content.
func (s *server) pinBzzRemovePinned(w http.ResponseWriter, r *http.Request) {
	addr, err := swarm.ParseHexAddress(mux.Vars(r)["address"])
	if err != nil {
		s.Logger.Debugf("pin bzz: parse address: %v", err)
		s.Logger.Error("pin bzz: parse address")
		jsonhttp.BadRequest(w, "bad address")
		return
	}

	has, err := s.Storer.Has(r.Context(), addr)
	if err != nil {
		s.Logger.Debugf("pin bzz: localstore has: %v", err)
		s.Logger.Error("pin bzz: store")
		jsonhttp.InternalServerError(w, err)
		return
	}

	if !has {
		jsonhttp.NotFound(w, nil)
		return
	}

	ctx := r.Context()

	chunkAddressFn := func(address swarm.Address) (stop bool) {
		_, err := s.Storer.PinCounter(address)
		if err != nil {
			return false
		}

		err = s.Storer.Set(r.Context(), storage.ModeSetUnpin, address)
		if err != nil {
			s.Logger.Debugf("pin bzz: unpinning error: %v, addr %s, for address: %s", err, addr, address)
			// continue un-pinning all chunks
		}

		return false
	}

	err = s.Traversal.TraverseManifestAddresses(ctx, addr, chunkAddressFn)
	if err != nil {
		s.Logger.Debugf("pin bzz: traverse chunks: %v, addr %s", err, addr)

		if errors.Is(err, traversal.ErrInvalidType) {
			s.Logger.Error("pin bzz: invalid type")
			jsonhttp.BadRequest(w, "invalid type")
			return
		}

		s.Logger.Error("pin bzz: cannot unpin")
		jsonhttp.InternalServerError(w, "cannot unpin")
		return
	}

	jsonhttp.OK(w, nil)
}
