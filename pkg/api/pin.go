// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/pinning"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/go-chi/chi/v5"
)

// pinRootHash pins root hash of given reference. This method is idempotent.
func (s *server) pinRootHash(w http.ResponseWriter, r *http.Request) {
	ref, err := swarm.ParseHexAddress(chi.URLParam(r, "reference"))
	if err != nil {
		s.logger.Debugf("pin root hash: unable to parse reference %q: %v", ref, err)
		s.logger.Error("pin root hash: unable to parse reference")
		jsonhttp.BadRequest(w, "bad reference")
		return
	}

	has, err := s.pinning.HasPin(ref)
	if err != nil {
		s.logger.Debugf("pin root hash: checking of tracking pin for %q failed: %v", ref, err)
		s.logger.Error("pin root hash: checking of tracking pin failed")
		jsonhttp.InternalServerError(w, nil)
		return
	}
	if has {
		jsonhttp.OK(w, nil)
		return
	}

	switch err = s.pinning.CreatePin(r.Context(), ref, true); {
	case errors.Is(err, storage.ErrNotFound):
		jsonhttp.NotFound(w, nil)
		return
	case err != nil:
		s.logger.Debugf("pin root hash: creation of tracking pin for %q failed: %v", ref, err)
		s.logger.Error("pin root hash: creation of tracking pin failed")
		jsonhttp.InternalServerError(w, nil)
		return
	}

	jsonhttp.Created(w, nil)
}

// unpinRootHash unpin's an already pinned root hash. This method is idempotent.
func (s *server) unpinRootHash(w http.ResponseWriter, r *http.Request) {
	ref, err := swarm.ParseHexAddress(chi.URLParam(r, "reference"))
	if err != nil {
		s.logger.Debugf("unpin root hash: unable to parse reference: %v", err)
		s.logger.Error("unpin root hash: unable to parse reference")
		jsonhttp.BadRequest(w, "bad reference")
		return
	}

	has, err := s.pinning.HasPin(ref)
	if err != nil {
		s.logger.Debugf("pin root hash: checking of tracking pin for %q failed: %v", ref, err)
		s.logger.Error("pin root hash: checking of tracking pin failed")
		jsonhttp.InternalServerError(w, nil)
		return
	}
	if !has {
		jsonhttp.NotFound(w, nil)
		return
	}

	switch err := s.pinning.DeletePin(r.Context(), ref); {
	case errors.Is(err, pinning.ErrTraversal):
		s.logger.Debugf("unpin root hash: deletion of pin for %q failed: %v", ref, err)
		jsonhttp.InternalServerError(w, nil)
		return
	case err != nil:
		s.logger.Debugf("unpin root hash: deletion of pin for %q failed: %v", ref, err)
		s.logger.Error("unpin root hash: deletion of pin for failed")
		jsonhttp.InternalServerError(w, nil)
		return
	}

	jsonhttp.OK(w, nil)
}

// getPinnedRootHash returns back the given reference if its root hash is pinned.
func (s *server) getPinnedRootHash(w http.ResponseWriter, r *http.Request) {
	ref, err := swarm.ParseHexAddress(chi.URLParam(r, "reference"))
	if err != nil {
		s.logger.Debugf("pinned root hash: unable to parse reference %q: %v", ref, err)
		s.logger.Error("pinned root hash: unable to parse reference")
		jsonhttp.BadRequest(w, "bad reference")
		return
	}

	has, err := s.pinning.HasPin(ref)
	if err != nil {
		s.logger.Debugf("pinned root hash: unable to check reference %q in the localstore: %v", ref, err)
		s.logger.Error("pinned root hash: unable to check reference in the localstore")
		jsonhttp.InternalServerError(w, nil)
		return
	}

	if !has {
		jsonhttp.NotFound(w, nil)
		return
	}

	jsonhttp.OK(w, struct {
		Reference swarm.Address `json:"reference"`
	}{
		Reference: ref,
	})
}

// listPinnedRootHashes lists all the references of the pinned root hashes.
func (s *server) listPinnedRootHashes(w http.ResponseWriter, r *http.Request) {
	pinned, err := s.pinning.Pins()
	if err != nil {
		s.logger.Debugf("list pinned root references: unable to list references: %v", err)
		s.logger.Error("list pinned root references: unable to list references")
		jsonhttp.InternalServerError(w, nil)
		return
	}

	jsonhttp.OK(w, struct {
		References []swarm.Address `json:"references"`
	}{
		References: pinned,
	})
}
