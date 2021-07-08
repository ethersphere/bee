// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strconv"

	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/gorilla/mux"
)

const (
	gasPriceHeader  = "Gas-Price"
	immutableHeader = "Immutable"
	errBadGasPrice  = "bad gas price"
)

type batchID []byte

func (b batchID) MarshalJSON() ([]byte, error) {
	return json.Marshal(hex.EncodeToString(b))
}

type postageCreateResponse struct {
	BatchID batchID `json:"batchID"`
}

func (s *server) postageCreateHandler(w http.ResponseWriter, r *http.Request) {
	depthStr := mux.Vars(r)["depth"]

	amount, ok := big.NewInt(0).SetString(mux.Vars(r)["amount"], 10)
	if !ok {
		s.logger.Error("create batch: invalid amount")
		jsonhttp.BadRequest(w, "invalid postage amount")
		return

	}

	depth, err := strconv.ParseUint(depthStr, 10, 8)
	if err != nil {
		s.logger.Debugf("create batch: invalid depth: %v", err)
		s.logger.Error("create batch: invalid depth")
		jsonhttp.BadRequest(w, "invalid depth")
		return
	}

	label := r.URL.Query().Get("label")

	ctx := r.Context()
	if price, ok := r.Header[gasPriceHeader]; ok {
		p, ok := big.NewInt(0).SetString(price[0], 10)
		if !ok {
			s.logger.Error("create batch: bad gas price")
			jsonhttp.BadRequest(w, errBadGasPrice)
			return
		}
		ctx = sctx.SetGasPrice(ctx, p)
	}

	var immutable bool
	if val, ok := r.Header[immutableHeader]; ok {
		immutable, _ = strconv.ParseBool(val[0])
	}

	batchID, err := s.postageContract.CreateBatch(ctx, amount, uint8(depth), immutable, label)
	if err != nil {
		if errors.Is(err, postagecontract.ErrInsufficientFunds) {
			s.logger.Debugf("create batch: out of funds: %v", err)
			s.logger.Error("create batch: out of funds")
			jsonhttp.BadRequest(w, "out of funds")
			return
		}
		if errors.Is(err, postagecontract.ErrInvalidDepth) {
			s.logger.Debugf("create batch: invalid depth: %v", err)
			s.logger.Error("create batch: invalid depth")
			jsonhttp.BadRequest(w, "invalid depth")
			return
		}
		s.logger.Debugf("create batch: failed to create: %v", err)
		s.logger.Error("create batch: failed to create")
		jsonhttp.InternalServerError(w, "cannot create batch")
		return
	}

	jsonhttp.Created(w, &postageCreateResponse{
		BatchID: batchID,
	})
}

type postageStampResponse struct {
	BatchID       batchID        `json:"batchID"`
	Utilization   uint32         `json:"utilization"`
	Usable        bool           `json:"usable"`
	Label         string         `json:"label"`
	Depth         uint8          `json:"depth"`
	Amount        *bigint.BigInt `json:"amount"`
	BucketDepth   uint8          `json:"bucketDepth"`
	BlockNumber   uint64         `json:"blockNumber"`
	ImmutableFlag bool           `json:"immutableFlag"`
}

type postageStampsResponse struct {
	Stamps []postageStampResponse `json:"stamps"`
}

func (s *server) postageGetStampsHandler(w http.ResponseWriter, _ *http.Request) {
	resp := postageStampsResponse{}
	for _, v := range s.post.StampIssuers() {
		resp.Stamps = append(resp.Stamps, postageStampResponse{
			BatchID:       v.ID(),
			Utilization:   v.Utilization(),
			Usable:        s.post.IssuerUsable(v),
			Label:         v.Label(),
			Depth:         v.Depth(),
			Amount:        bigint.Wrap(v.Amount()),
			BucketDepth:   v.BucketDepth(),
			BlockNumber:   v.BlockNumber(),
			ImmutableFlag: v.ImmutableFlag(),
		})
	}
	jsonhttp.OK(w, resp)
}

func (s *server) postageGetStampHandler(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	if idStr == "" || len(idStr) != 64 {
		s.logger.Error("get stamp issuer: invalid batchID")
		jsonhttp.BadRequest(w, "invalid batchID")
		return
	}
	id, err := hex.DecodeString(idStr)
	if err != nil {
		s.logger.Error("get stamp issuer: invalid batchID: %v", err)
		s.logger.Error("get stamp issuer: invalid batchID")
		jsonhttp.BadRequest(w, "invalid batchID")
		return
	}

	issuer, err := s.post.GetStampIssuer(id)
	if err != nil {
		s.logger.Error("get stamp issuer: get issuer: %v", err)
		s.logger.Error("get stamp issuer: get issuer")
		jsonhttp.BadRequest(w, "cannot get issuer")
		return
	}
	resp := postageStampResponse{
		BatchID:       id,
		Utilization:   issuer.Utilization(),
		Usable:        s.post.IssuerUsable(issuer),
		Label:         issuer.Label(),
		Depth:         issuer.Depth(),
		Amount:        bigint.Wrap(issuer.Amount()),
		BucketDepth:   issuer.BucketDepth(),
		BlockNumber:   issuer.BlockNumber(),
		ImmutableFlag: issuer.ImmutableFlag(),
	}
	jsonhttp.OK(w, &resp)
}
