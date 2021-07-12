// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

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

type batchID []byte

func (b batchID) MarshalJSON() ([]byte, error) {
	return json.Marshal(hex.EncodeToString(b))
}

type postageCreateResponse struct {
	BatchID batchID `json:"batchID"`
}

func (s *Service) postageCreateHandler(w http.ResponseWriter, r *http.Request) {
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

type postageStampBucketsResponse struct {
	Depth            uint8        `json:"depth"`
	BucketDepth      uint8        `json:"bucketDepth"`
	BucketUpperBound uint32       `json:"bucketUpperBound"`
	Buckets          []bucketData `json:"buckets"`
}

type bucketData struct {
	BucketID   uint32 `json:"bucketID"`
	Collisions uint32 `json:"collisions"`
}

func (s *Service) postageGetStampsHandler(w http.ResponseWriter, _ *http.Request) {
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

func (s *Service) postageGetStampBucketsHandler(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	if len(idStr) != 64 {
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
		jsonhttp.BadRequest(w, "cannot get batch")
		return
	}

	b := issuer.Buckets()
	resp := postageStampBucketsResponse{
		Depth:            issuer.Depth(),
		BucketDepth:      issuer.BucketDepth(),
		BucketUpperBound: issuer.BucketUpperBound(),
		Buckets:          make([]bucketData, len(b)),
	}

	for i, v := range b {
		resp.Buckets[i] = bucketData{BucketID: uint32(i), Collisions: v}
	}

	jsonhttp.OK(w, resp)
}

func (s *Service) postageGetStampHandler(w http.ResponseWriter, r *http.Request) {
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

type reserveStateResponse struct {
	Radius        uint8          `json:"radius"`
	StorageRadius uint8          `json:"storageRadius"`
	Available     int64          `json:"available"`
	Outer         *bigint.BigInt `json:"outer"` // lower value limit for outer layer = the further half of chunks
	Inner         *bigint.BigInt `json:"inner"`
}

type chainStateResponse struct {
	Block        uint64         `json:"block"`        // The block number of the last postage event.
	TotalAmount  *bigint.BigInt `json:"totalAmount"`  // Cumulative amount paid per stamp.
	CurrentPrice *bigint.BigInt `json:"currentPrice"` // Bzz/chunk/block normalised price.
}

func (s *Service) reserveStateHandler(w http.ResponseWriter, _ *http.Request) {
	state := s.batchStore.GetReserveState()

	jsonhttp.OK(w, reserveStateResponse{
		Radius:    state.Radius,
		Available: state.Available,
		Outer:     bigint.Wrap(state.Outer),
		Inner:     bigint.Wrap(state.Inner),
	})
}

// chainStateHandler returns the current chain state.
func (s *Service) chainStateHandler(w http.ResponseWriter, _ *http.Request) {
	state := s.batchStore.GetChainState()

	jsonhttp.OK(w, chainStateResponse{
		Block:        state.Block,
		TotalAmount:  bigint.Wrap(state.TotalAmount),
		CurrentPrice: bigint.Wrap(state.CurrentPrice),
	})
}
