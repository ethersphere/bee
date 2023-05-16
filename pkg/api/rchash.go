// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package api

import (
	"encoding/hex"
	"net/http"
	"time"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/gorilla/mux"
)

// TODO: Remove this API before next release. This API is kept mainly for testing
// the sampler till the storage incentives agent falls into place. As a result,
// no documentation or tests are added here. This should be removed before next
// breaking release.
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

	start := time.Now()

	sample, proofs, err := s.redistributionAgent.SampleWithProofs(r.Context(), anchor1, anchor2, paths.Depth)
	if err != nil {
		logger.Error(err, "failed making sample with proofs")
		jsonhttp.InternalServerError(w, "failed making sample with proofs")
		return
	}

	jsonhttp.OK(w, map[string]any{
		"sample": *sample,
		"proofs": *proofs,
		"time":   time.Since(start).String(),
	})
}
