// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package api

import (
	"encoding/hex"
	"net/http"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storageincentives"
	"github.com/gorilla/mux"
)

type RCHashResponse storageincentives.SampleWithProofs

// This API is kept for testing the sampler. As a result, no documentation or tests are added here.
func (s *Service) rchash(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_rchash").Build()

	paths := struct {
		Depth   uint8  `map:"depth" validate:"required"`
		Anchor1 string `map:"anchor1" validate:"required"`
		Anchor2 string `map:"anchor2" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	anchor1, err := hex.DecodeString(paths.Anchor1)
	if err != nil {
		logger.Error(err, "invalid hex params")
		jsonhttp.InternalServerError(w, "invalid hex params")
		return
	}
	anchor2, err := hex.DecodeString(paths.Anchor2)
	if err != nil {
		logger.Error(err, "invalid hex params")
		jsonhttp.InternalServerError(w, "invalid hex params")
		return
	}

	resp, err := s.redistributionAgent.SampleWithProofs(r.Context(), anchor1, anchor2, paths.Depth)
	if err != nil {
		logger.Error(err, "failed making sample with proofs")
		jsonhttp.InternalServerError(w, "failed making sample with proofs")
		return
	}

	jsonhttp.OK(w, RCHashResponse(resp))
}
