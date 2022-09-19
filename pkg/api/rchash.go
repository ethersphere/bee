// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	//"encoding/json"
	// "errors"
	// "io"
	"net/http"
	"strconv"
	"time"

	"github.com/ethersphere/bee/pkg/commitment"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

type rchash struct {
	Hash        swarm.Address           `json:"hash"`
	Items       []commitment.SampleItem `json:"items"`
	ReserveSize uint64                  `json:"reserveSize"`
	Iterated    uint64
	Doubled     uint64
	Errored     uint64
	Time        string
}

func (s *Service) rchasher(w http.ResponseWriter, r *http.Request) {

	start := time.Now()
	depthStr := mux.Vars(r)["depth"]

	depth, err := strconv.ParseUint(depthStr, 10, 8)
	if err != nil {
		s.logger.Debug("reserve commitment hasher parse depth", "error", err)
		s.logger.Error(err, "reserve commitment hasher: invalid depth")
		jsonhttp.BadRequest(w, "invalid depth")
		return
	}

	if depth > 255 {
		depth = 255
	}

	anchorStr := mux.Vars(r)["anchor"]
	anchor := []byte(anchorStr)

	rec, iter, doub, errd, reserveSize, err := s.hasher.MakeSample(r.Context(), anchor, uint8(depth))
	if err != nil {
		jsonhttp.BadRequest(w, "invalid")
		return
	}

	jsonhttp.OK(w, rchash{
		Hash:        rec.Hash,
		Items:       rec.Items,
		ReserveSize: reserveSize,
		Iterated:    iter,
		Doubled:     doub,
		Errored:     errd,
		Time:        time.Since(start).String(),
	})
}
