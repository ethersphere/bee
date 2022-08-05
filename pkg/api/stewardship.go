// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/gorilla/mux"
)

//  stewardshipPutHandler re-uploads root hash and all of its underlying
// associated chunks to the network.
func (s *Service) stewardshipPutHandler(w http.ResponseWriter, r *http.Request) {
	nameOrHex := mux.Vars(r)["address"]
	address, err := s.resolveNameOrAddress(nameOrHex)
	if err != nil {
		s.logger.Debug("stewardship put: parse address string failed", "string", nameOrHex, "error", err)
		s.logger.Error(nil, "stewardship put: parse address string failed")
		jsonhttp.NotFound(w, nil)
		return
	}
	err = s.steward.Reupload(r.Context(), address)
	if err != nil {
		s.logger.Debug("stewardship put: re-upload failed", "address", address, "error", err)
		s.logger.Error(nil, "stewardship put: re-upload failed")
		jsonhttp.InternalServerError(w, "stewardship put: re-upload failed")
		return
	}
	jsonhttp.OK(w, nil)
}

type isRetrievableResponse struct {
	IsRetrievable bool `json:"isRetrievable"`
}

// stewardshipGetHandler checks whether the content on the given address is retrievable.
func (s *Service) stewardshipGetHandler(w http.ResponseWriter, r *http.Request) {
	nameOrHex := mux.Vars(r)["address"]
	address, err := s.resolveNameOrAddress(nameOrHex)
	if err != nil {
		s.logger.Debug("stewardship get: parse address string failed", "string", nameOrHex, "error", err)
		s.logger.Error(nil, "stewardship get: parse address string failed")
		jsonhttp.NotFound(w, nil)
		return
	}
	res, err := s.steward.IsRetrievable(r.Context(), address)
	if err != nil {
		s.logger.Debug("stewardship get: is retrievable check failed", "address", address, "error", err)
		s.logger.Error(nil, "stewardship get: is retrievable")
		jsonhttp.InternalServerError(w, "stewardship get: is retrievable check failed")
		return
	}
	jsonhttp.OK(w, isRetrievableResponse{
		IsRetrievable: res,
	})
}
