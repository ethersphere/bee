// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"errors"
	"net/http"

	"github.com/ethersphere/bee/pkg/resolver"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/gorilla/mux"
)

// StewardshipPutHandler re-uploads root hash and all of its underlying associated chunks to the network.
func (s *Service) stewardshipPutHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("put_stewardship").Build()

	paths := struct {
		Address string `map:"address" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	// TODO: normalize the response errors as in validation case.
	address, err := s.resolveNameOrAddress(paths.Address)
	switch {
	case errors.Is(err, resolver.ErrParse), errors.Is(err, resolver.ErrInvalidContentHash):
		logger.Debug("mapStructure address string failed", "string", paths.Address, "error", err)
		logger.Error(nil, "invalid address")
		jsonhttp.BadRequest(w, "invalid address")
		return
	case errors.Is(err, resolver.ErrNotFound):
		logger.Debug("address not found", "string", paths.Address, "error", err)
		logger.Error(nil, "address not found")
		jsonhttp.NotFound(w, "address not found")
		return
	case errors.Is(err, resolver.ErrServiceNotAvailable):
		logger.Debug("service unavailable", "string", paths.Address, "error", err)
		logger.Error(nil, "service unavailable")
		jsonhttp.InternalServerError(w, "resolver service unavailable")
		return
	case err != nil:
		logger.Debug("resolve address or name string failed", "string", paths.Address, "error", err)
		logger.Error(nil, "resolve address or name string failed")
		jsonhttp.InternalServerError(w, "resolve name or address")
		return
	}
	err = s.steward.Reupload(r.Context(), address)
	if err != nil {
		logger.Debug("re-upload failed", "chunk_address", address, "error", err)
		logger.Error(nil, "re-upload failed")
		jsonhttp.InternalServerError(w, "re-upload failed")
		return
	}
	jsonhttp.OK(w, nil)
}

type isRetrievableResponse struct {
	IsRetrievable bool `json:"isRetrievable"`
}

// stewardshipGetHandler checks whether the content on the given address is retrievable.
func (s *Service) stewardshipGetHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_stewardship").Build()

	paths := struct {
		Address string `map:"address" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	// TODO: normalize the response errors as in validation case.
	address, err := s.resolveNameOrAddress(paths.Address)
	if err != nil {
		logger.Debug("mapStructure address string failed", "string", paths.Address, "error", err)
		logger.Error(nil, "mapStructure address string failed")
		jsonhttp.NotFound(w, nil)
		return
	}
	res, err := s.steward.IsRetrievable(r.Context(), address)
	if err != nil {
		logger.Debug("is retrievable check failed", "chunk_address", address, "error", err)
		logger.Error(nil, "is retrievable")
		jsonhttp.InternalServerError(w, "is retrievable check failed")
		return
	}
	jsonhttp.OK(w, isRetrievableResponse{
		IsRetrievable: res,
	})
}
