// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/traversal"
	"github.com/gorilla/mux"
)

// pinRootHash pins root hash of given reference. This method is idempotent.
func (s *Service) pinRootHash(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("post_pin").Build()

	paths := struct {
		Reference swarm.Address `map:"reference" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	has, err := s.storer.HasPin(paths.Reference)
	if err != nil {
		logger.Debug("unable to check pin", "chunk_address", paths.Reference, "error", err)
		logger.Warning("unable to check pin")
		jsonhttp.InternalServerError(w, "check pin failed")
		return
	}
	if has {
		jsonhttp.OK(w, nil)
		return
	}

	putter, err := s.storer.NewCollection(r.Context())
	if err != nil {
		logger.Debug("unable to create collection", "error", err)
		logger.Warning("unable to create collection")
		jsonhttp.InternalServerError(w, "create collection failed")
		return
	}

	getter := s.storer.Download(true)
	traverser := traversal.New(getter)

	err = traverser.Traverse(
		r.Context(),
		paths.Reference,
		func(address swarm.Address) error {
			chunk, err := getter.Get(r.Context(), address)
			if err != nil {
				return err
			}
			err = putter.Put(r.Context(), chunk)
			if err != nil {
				return err
			}
			return nil
		},
	)
	if err != nil {
		logger.Debug("unable to pin collection", "error", errors.Join(err, putter.Cleanup()))
		logger.Warning("unable to pin collection")
		jsonhttp.InternalServerError(w, "pin collection failed")
		return
	}

	err = putter.Done(paths.Reference)
	if err != nil {
		logger.Debug("unable to close session", "error", err)
		logger.Warning("unable to close session")
		jsonhttp.InternalServerError(w, "unable to close session")
		return
	}

	jsonhttp.Created(w, nil)
}

// unpinRootHash unpin's an already pinned root hash. This method is idempotent.
func (s *Service) unpinRootHash(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("delete_pin").Build()

	paths := struct {
		Reference swarm.Address `map:"reference" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	has, err := s.storer.HasPin(paths.Reference)
	if err != nil {
		logger.Debug("unable to check pin", "chunk_address", paths.Reference, "error", err)
		logger.Warning("unable to check pin")
		jsonhttp.InternalServerError(w, "check pin failed")
		return
	}
	if !has {
		jsonhttp.NotFound(w, nil)
		return
	}

	if err := s.storer.DeletePin(r.Context(), paths.Reference); err != nil {
		logger.Debug("unable to delete pin", "chunk_address", paths.Reference, "error", err)
		logger.Warning("unable to delete pin")
		jsonhttp.InternalServerError(w, "deletion of pin failed")
		return
	}

	jsonhttp.OK(w, nil)
}

// getPinnedRootHash returns back the given reference if its root hash is pinned.
func (s *Service) getPinnedRootHash(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_pin").Build()

	paths := struct {
		Reference swarm.Address `map:"reference" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	has, err := s.storer.HasPin(paths.Reference)
	if err != nil {
		logger.Debug("unable to check pin", "chunk_address", paths.Reference, "error", err)
		logger.Warning("unable to check pin")
		jsonhttp.InternalServerError(w, "check pin failed")
		return
	}

	if !has {
		jsonhttp.NotFound(w, nil)
		return
	}

	jsonhttp.OK(w, struct {
		Reference swarm.Address `json:"reference"`
	}{
		Reference: paths.Reference,
	})
}

// listPinnedRootHashes lists all the references of the pinned root hashes.
func (s *Service) listPinnedRootHashes(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_pins").Build()

	pinned, err := s.storer.Pins()
	if err != nil {
		logger.Debug("unable to list pins", "error", err)
		logger.Warning("unable to list pins")
		jsonhttp.InternalServerError(w, "list pinned root references failed")
		return
	}

	jsonhttp.OK(w, struct {
		References []swarm.Address `json:"references"`
	}{
		References: pinned,
	})
}
