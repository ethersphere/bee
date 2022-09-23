// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"github.com/gorilla/mux"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// pinRootHash pins root hash of given reference. This method is idempotent.
func (s *Service) pinRootHash(w http.ResponseWriter, r *http.Request) {
	path := struct {
		Reference []byte `parse:"reference,addressToBytes" name:"reference" errMessage:"parse reference string failed"`
	}{}

	if err := s.parseAndValidate(mux.Vars(r), &path); err != nil {
		s.logger.Debug("pin root hash: parse reference string failed", "string", mux.Vars(r)["reference"], "error", err)
		s.logger.Error(nil, "pin root hash: parse reference string failed")
		jsonhttp.BadRequest(w, err.Error())
		return
	}

	swarmAdr := swarm.NewAddress(path.Reference)
	has, err := s.pinning.HasPin(swarmAdr)
	if err != nil {
		s.logger.Debug("pin root hash: has pin failed", "chunk_address", swarmAdr, "error", err)
		s.logger.Error(nil, "pin root hash: has pin failed")
		jsonhttp.InternalServerError(w, "pin root hash: checking of tracking pin failed")
		return
	}
	if has {
		jsonhttp.OK(w, nil)
		return
	}

	switch err = s.pinning.CreatePin(r.Context(), swarmAdr, true); {
	case errors.Is(err, storage.ErrNotFound):
		jsonhttp.NotFound(w, nil)
		return
	case err != nil:
		s.logger.Debug("pin root hash: create pin failed", "chunk_address", swarmAdr, "error", err)
		s.logger.Error(nil, "pin root hash: create pin failed")
		jsonhttp.InternalServerError(w, "pin root hash: creation of tracking pin failed")
		return
	}

	jsonhttp.Created(w, nil)
}

// unpinRootHash unpin's an already pinned root hash. This method is idempotent.
func (s *Service) unpinRootHash(w http.ResponseWriter, r *http.Request) {
	path := struct {
		Reference []byte `parse:"reference,addressToBytes" name:"reference" errMessage:"parse reference string failed"`
	}{}

	if err := s.parseAndValidate(mux.Vars(r), &path); err != nil {
		s.logger.Debug("unpin root hash: parse reference string failed", "string", mux.Vars(r)["reference"], "error", err)
		s.logger.Error(nil, "unpin root hash: parse reference string failed")
		jsonhttp.BadRequest(w, err.Error())
		return
	}

	swarmAdr := swarm.NewAddress(path.Reference)
	has, err := s.pinning.HasPin(swarmAdr)
	if err != nil {
		s.logger.Debug("unpin root hash: has pin failed", "chunk_address", swarmAdr, "error", err)
		s.logger.Error(nil, "unpin root hash: has pin failed")
		jsonhttp.InternalServerError(w, "pin root hash: checking of tracking pin")
		return
	}
	if !has {
		jsonhttp.NotFound(w, nil)
		return
	}

	if err := s.pinning.DeletePin(r.Context(), swarmAdr); err != nil {
		s.logger.Debug("unpin root hash: delete pin failed", "chunk_address", swarmAdr, "error", err)
		s.logger.Error(nil, "unpin root hash: delete pin failed")
		jsonhttp.InternalServerError(w, "unpin root hash: deletion of pin failed")
		return
	}

	jsonhttp.OK(w, nil)
}

// getPinnedRootHash returns back the given reference if its root hash is pinned.
func (s *Service) getPinnedRootHash(w http.ResponseWriter, r *http.Request) {
	path := struct {
		Reference []byte `parse:"reference,addressToBytes" name:"reference" errMessage:"parse reference string failed"`
	}{}

	if err := s.parseAndValidate(mux.Vars(r), &path); err != nil {
		s.logger.Debug("pinned root hash: parse reference string failed", "string", mux.Vars(r)["reference"], "error", err)
		s.logger.Error(nil, "pinned root hash: parse reference string failed")
		jsonhttp.BadRequest(w, err.Error())
		return
	}
	swarnAdr := swarm.NewAddress(path.Reference)
	has, err := s.pinning.HasPin(swarnAdr)
	if err != nil {
		s.logger.Debug("pinned root hash: has pin failed", "chunk_address", swarnAdr, "error", err)
		s.logger.Error(nil, "pinned root hash: has pin failed")
		jsonhttp.InternalServerError(w, "pinned root hash: check reference failed")
		return
	}

	if !has {
		jsonhttp.NotFound(w, nil)
		return
	}

	jsonhttp.OK(w, struct {
		Reference swarm.Address `json:"reference"`
	}{
		Reference: swarnAdr,
	})
}

// listPinnedRootHashes lists all the references of the pinned root hashes.
func (s *Service) listPinnedRootHashes(w http.ResponseWriter, r *http.Request) {
	pinned, err := s.pinning.Pins()
	if err != nil {
		s.logger.Debug("list pinned root references: unable to list references", "error", err)
		s.logger.Error(nil, "list pinned root references: unable to list references")
		jsonhttp.InternalServerError(w, "list pinned root references failed")
		return
	}

	jsonhttp.OK(w, struct {
		References []swarm.Address `json:"references"`
	}{
		References: pinned,
	})
}
