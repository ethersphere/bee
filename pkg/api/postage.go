// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/hex"
	"math/big"
	"net/http"
	"strconv"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/gorilla/mux"
)

func (s *server) postageCreateHandler(w http.ResponseWriter, r *http.Request) {
	depthStr := mux.Vars(r)["depth"]

	amount, ok := big.NewInt(0).SetString(mux.Vars(r)["amount"], 10)
	if !ok {
		jsonhttp.BadRequest(w, "invalid postage amount")
		s.Logger.Error("api: invalid postage amount")
		return
	}

	depth, err := strconv.ParseUint(depthStr, 10, 8)
	if err != nil {
		jsonhttp.InternalServerError(w, "invalid depth")
		s.Logger.Error("api: invalid depth")
		return
	}

	batchID, err := s.postageContract.CreateBatch(r.Context(), amount, uint8(depth))
	if err != nil {
		jsonhttp.InternalServerError(w, "cannot create batch")
		s.Logger.Error("api: cannot create batch")
		s.Logger.Debugf("api: cannot create batch: %v", err)
		return
	}

	jsonhttp.OK(w, hex.EncodeToString(batchID))
}
