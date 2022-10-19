// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/swarm"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/gorilla/mux"
)

// StewardshipPutHandler re-uploads root hash and all of its underlying associated chunks to the network.
func (s *Service) stewardshipPutHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("put_stewardship").Build()

	paths := struct {
		Address swarm.Address `map:"address,resolve" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	err := s.steward.Reupload(r.Context(), paths.Address)
	if err != nil {
		logger.Debug("re-upload failed", "chunk_address", paths.Address, "error", err)
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
		Address swarm.Address `map:"address,resolve" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	res, err := s.steward.IsRetrievable(r.Context(), paths.Address)
	if err != nil {
		logger.Debug("is retrievable check failed", "chunk_address", paths.Address, "error", err)
		logger.Error(nil, "is retrievable")
		jsonhttp.InternalServerError(w, "is retrievable check failed")
		return
	}
	jsonhttp.OK(w, isRetrievableResponse{
		IsRetrievable: res,
	})
}
