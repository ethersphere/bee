// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/pinning"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

// pinRootHash pins root hash of given address. This method is idempotent.
func (s *server) pinRootHash(w http.ResponseWriter, r *http.Request) {
	addr, err := swarm.ParseHexAddress(mux.Vars(r)["address"])
	if err != nil {
		s.logger.Debugf("pin root hash: unable to parse address %q: %v", addr, err)
		s.logger.Error("pin root hash: unable to parse address")
		jsonhttp.BadRequest(w, "bad address")
		return
	}

	has, err := s.pinning.HasPin(addr)
	if err != nil {
		s.logger.Debugf("pin root hash: checking of tracking pin for %q failed: %v", addr, err)
		s.logger.Error("pin root hash: checking of tracking pin failed")
		jsonhttp.InternalServerError(w, nil)
		return
	}
	if has {
		jsonhttp.OK(w, nil)
		return
	}

	err = s.pinning.CreatePin(r.Context(), addr, true)
	if err != nil {
		s.logger.Debugf("pin root hash: creation of tracking pin for %q failed: %v", addr, err)
		s.logger.Error("pin root hash: creation of tracking pin failed")
		jsonhttp.InternalServerError(w, nil)
		return
	}

	jsonhttp.Created(w, nil)
}

// unpinRootHash unpin's an already pinned root hash. This method is idempotent.
func (s *server) unpinRootHash(w http.ResponseWriter, r *http.Request) {
	addr, err := swarm.ParseHexAddress(mux.Vars(r)["address"])
	if err != nil {
		s.logger.Debugf("unpin root hash: unable to parse address: %v", err)
		s.logger.Error("unpin root hash: unable to parse address")
		jsonhttp.BadRequest(w, "bad address")
		return
	}

	has, err := s.pinning.HasPin(addr)
	if err != nil {
		s.logger.Debugf("pin root hash: checking of tracking pin for %q failed: %v", addr, err)
		s.logger.Error("pin root hash: checking of tracking pin failed")
		jsonhttp.InternalServerError(w, nil)
		return
	}
	if !has {
		jsonhttp.NotFound(w, nil)
		return
	}

	switch err := s.pinning.DeletePin(r.Context(), addr); {
	case errors.Is(err, pinning.ErrTraversal):
		s.logger.Debugf("unpin root hash: deletion of pin for %q failed: %v", addr, err)
		jsonhttp.InternalServerError(w, nil)
		return
	case err != nil:
		s.logger.Debugf("unpin root hash: deletion of pin for %q failed: %v", addr, err)
		s.logger.Error("unpin root hash: deletion of pin for failed")
		jsonhttp.InternalServerError(w, nil)
		return
	}

	jsonhttp.OK(w, nil)
}

// getPinnedRootHash returns back the given address if its root hash is pinned.
func (s *server) getPinnedRootHash(w http.ResponseWriter, r *http.Request) {
	addr, err := swarm.ParseHexAddress(mux.Vars(r)["address"])
	if err != nil {
		s.logger.Debugf("pinned root hash: unable to parse address %q: %v", addr, err)
		s.logger.Error("pinned root hash: unable to parse address")
		jsonhttp.BadRequest(w, "bad address")
		return
	}

	has, err := s.pinning.HasPin(addr)
	if err != nil {
		s.logger.Debugf("pinned root hash: unable to check address %q in the localstore: %v", addr, err)
		s.logger.Error("pinned root hash: unable to check address in the localstore")
		jsonhttp.InternalServerError(w, nil)
		return
	}

	if !has {
		jsonhttp.NotFound(w, nil)
		return
	}

	jsonhttp.OK(w, struct {
		Address swarm.Address `json:"address"`
	}{
		Address: addr,
	})
}

// listPinnedRootHashes lists all the address of the pinned root hashes..
func (s *server) listPinnedRootHashes(w http.ResponseWriter, r *http.Request) {
	pinned, err := s.pinning.Pins()
	if err != nil {
		s.logger.Debugf("list pinned root addresses: unable to list addresses: %v", err)
		s.logger.Error("list pinned root addresses: unable to list addresses")
		jsonhttp.InternalServerError(w, nil)
		return
	}

	jsonhttp.OK(w, struct {
		Addresses []swarm.Address `json:"addresses"`
	}{
		Addresses: pinned,
	})
}
