// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"net/http"
	"strconv"
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

	start := time.Now()
	depthStr := mux.Vars(r)["depth"]

	depth, err := strconv.ParseUint(depthStr, 10, 8)
	if err != nil {
		s.logger.Error(err, "reserve commitment hasher: invalid depth")
		jsonhttp.BadRequest(w, "invalid depth")
		return
	}

	if depth > 255 {
		depth = 255
	}

	anchorStr := mux.Vars(r)["anchor"]
	anchor := []byte(anchorStr)

	sample, err := s.storer.ReserveSample(r.Context(), anchor, uint8(depth))
	if err != nil {
		s.logger.Error(err, "reserve commitment hasher: failed generating sample")
		jsonhttp.InternalServerError(w, "failed generating sample")
		return
	}

	jsonhttp.OK(w, rchash{
		Sample: sample,
		Time:   time.Since(start).String(),
	})
}
