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
func (s *server) stewardshipPutHandler(w http.ResponseWriter, r *http.Request) {
	nameOrHex := mux.Vars(r)["address"]
	address, err := s.resolveNameOrAddress(nameOrHex)
	if err != nil {
		s.logger.Debugf("stewardship put: parse address %s: %v", nameOrHex, err)
		s.logger.Error("stewardship put: parse address")
		jsonhttp.NotFound(w, nil)
		return
	}
	err = s.steward.Reupload(r.Context(), address)
	if err != nil {
		s.logger.Debugf("stewardship put: re-upload %s: %v", address, err)
		s.logger.Error("stewardship put: re-upload")
		jsonhttp.InternalServerError(w, nil)
		return
	}
	jsonhttp.OK(w, nil)
}

// stewardshipGetHandler checks whether the content on the given address is retrievable.
func (s *server) stewardshipGetHandler(w http.ResponseWriter, r *http.Request) {
	nameOrHex := mux.Vars(r)["address"]
	address, err := s.resolveNameOrAddress(nameOrHex)
	if err != nil {
		s.logger.Debugf("stewardship get: parse address %s: %v", nameOrHex, err)
		s.logger.Error("stewardship get: parse address")
		jsonhttp.NotFound(w, nil)
		return
	}
	res, err := s.steward.IsRetrievable(r.Context(), address)
	if err != nil {
		s.logger.Debugf("stewardship get: is retrievable %s: %v", address, err)
		s.logger.Error("stewardship get: is retrievable")
		jsonhttp.InternalServerError(w, nil)
		return
	}
	jsonhttp.OK(w, struct {
		IsRetrievable bool `json:"isRetrievable"`
	}{
		IsRetrievable: res,
	})
}
