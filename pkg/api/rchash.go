// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"math/big"
	"net/http"
	"time"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/gorilla/mux"
)

type rchash struct {
	Sample storage.Sample
	Time   string
}

// TODO: Remove this API before next release. This API is kept mainly for testing
// the sampler till the storage incentives agent falls into place. As a result,
// no documentation or tests are added here. This should be removed before next
// breaking release.
func (s *Service) rchasher(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_rchash").Build()

	paths := struct {
		Depth  uint8  `map:"depth" validate:"required"`
		Anchor string `map:"anchor" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	start := time.Now()
	sample, err := s.storer.ReserveSample(r.Context(), []byte(paths.Anchor), paths.Depth, uint64(start.UnixNano()), big.NewInt(0))
	if err != nil {
		logger.Error(err, "reserve commitment hasher: failed generating sample")
		jsonhttp.InternalServerError(w, "failed generating sample")
		return
	}

	jsonhttp.OK(w, rchash{
		Sample: sample,
		Time:   time.Since(start).String(),
	})
}
