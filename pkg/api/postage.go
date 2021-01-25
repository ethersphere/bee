// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"math/big"
	"net/http"
	"strconv"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/gorilla/mux"
)

type postageCreateResponse struct {
	BatchID []byte `json:"batchID"`
}

func (s *server) postageCreateHandler(w http.ResponseWriter, r *http.Request) {
	depthStr := mux.Vars(r)["depth"]

	amount, ok := big.NewInt(0).SetString(mux.Vars(r)["amount"], 10)
	if !ok {
		s.Logger.Error("create batch: invalid amount")
		jsonhttp.BadRequest(w, "invalid postage amount")
		return

	}

	depth, err := strconv.ParseUint(depthStr, 10, 8)
	if err != nil {
		s.Logger.Debugf("create batch: invalid depth: %v", err)
		s.Logger.Error("create batch: invalid depth")
		jsonhttp.BadRequest(w, "invalid depth")
		return
	}

	batchID, err := s.postageContract.CreateBatch(r.Context(), amount, uint8(depth))
	if err != nil {
		s.Logger.Debugf("create batch: failed to create: %v", err)
		s.Logger.Error("create batch: failed to create")
		jsonhttp.InternalServerError(w, "cannot create batch")
		return
	}

	jsonhttp.OK(w, &postageCreateResponse{
		BatchID: batchID,
	})
}
