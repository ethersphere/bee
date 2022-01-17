// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"strconv"

	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/gorilla/mux"
)

func (s *Service) postageAccessHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.postageSem.TryAcquire(1) {
			s.logger.Debug("postage access: simultaneous on-chain operations not supported")
			s.logger.Error("postage access: simultaneous on-chain operations not supported")
			jsonhttp.TooManyRequests(w, "simultaneous on-chain operations not supported")
			return
		}
		defer s.postageSem.Release(1)

		h.ServeHTTP(w, r)
	})
}

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
	Exists        bool           `json:"exists"`
	BatchTTL      int64          `json:"batchTTL"`
}

type postageStampsResponse struct {
	Stamps []postageStampResponse `json:"stamps"`
}

type postageBatchResponse struct {
	BatchID     batchID  `json:"batchID"`
	Value       *big.Int `json:"value"`
	Start       uint64   `json:"start"`
	Owner       batchID  `json:"owner"`
	Depth       uint8    `json:"depth"`
	BucketDepth uint8    `json:"bucketDepth"`
	Immutable   bool     `json:"immutable"`
	Radius      uint8    `json:"radius"`
	BatchTTL    int64    `json:"batchTTL"`
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

func (s *Service) postageGetStampsHandler(w http.ResponseWriter, r *http.Request) {
	resp := postageStampsResponse{}
	for _, v := range s.post.StampIssuers() {
		exists, err := s.batchStore.Exists(v.ID())
		if err != nil {
			s.logger.Debugf("get stamp issuer: check batch: %v", err)
			s.logger.Error("get stamp issuer: check batch")
			jsonhttp.InternalServerError(w, "unable to check batch")
			return
		}
		batchTTL, err := s.estimateBatchTTLFromID(v.ID())
		if err != nil {
			s.logger.Debugf("get stamp issuer: estimate batch expiration: %v", err)
			s.logger.Error("get stamp issuer: estimate batch expiration")
			jsonhttp.InternalServerError(w, "unable to estimate batch expiration")
			return
		}
		resp.Stamps = append(resp.Stamps, postageStampResponse{
			BatchID:       v.ID(),
			Utilization:   v.Utilization(),
			Usable:        exists && s.post.IssuerUsable(v),
			Label:         v.Label(),
			Depth:         v.Depth(),
			Amount:        bigint.Wrap(v.Amount()),
			BucketDepth:   v.BucketDepth(),
			BlockNumber:   v.BlockNumber(),
			ImmutableFlag: v.ImmutableFlag(),
			Exists:        exists,
			BatchTTL:      batchTTL,
		})
	}
	jsonhttp.OK(w, resp)
}

func (s *Service) postageGetAllStampsHandler(w http.ResponseWriter, _ *http.Request) {
	batches := []postageBatchResponse{}
	err := s.batchStore.Iterate(func(b *postage.Batch) (bool, error) {
		batchTTL, err := s.estimateBatchTTL(b)
		if err != nil {
			return false, fmt.Errorf("estimate batch ttl: %w", err)
		}

		batches = append(batches, postageBatchResponse{
			BatchID:     b.ID,
			Value:       b.Value,
			Start:       b.Start,
			Owner:       b.Owner,
			Depth:       b.Depth,
			BucketDepth: b.BucketDepth,
			Immutable:   b.Immutable,
			Radius:      b.Radius,
			BatchTTL:    batchTTL,
		})
		return false, nil
	})
	if err != nil {
		s.logger.Debugf("iterate batches: %v", err)
		s.logger.Error("iterate batches: cannot complete operation")
		jsonhttp.InternalServerError(w, "unable to iterate all batches")
		return
	}

	jsonhttp.OK(w, batches)
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
		s.logger.Debugf("get stamp issuer: invalid batchID: %v", err)
		s.logger.Error("get stamp issuer: invalid batchID")
		jsonhttp.BadRequest(w, "invalid batchID")
		return
	}

	issuer, err := s.post.GetStampIssuer(id)
	if err != nil {
		s.logger.Debugf("get stamp issuer: get issuer: %v", err)
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
	if len(idStr) != 64 {
		s.logger.Error("get stamp issuer: invalid batchID")
		jsonhttp.BadRequest(w, "invalid batchID")
		return
	}
	id, err := hex.DecodeString(idStr)
	if err != nil {
		s.logger.Debugf("get stamp issuer: invalid batchID: %v", err)
		s.logger.Error("get stamp issuer: invalid batchID")
		jsonhttp.BadRequest(w, "invalid batchID")
		return
	}

	issuer, err := s.post.GetStampIssuer(id)
	if err != nil && !errors.Is(err, postage.ErrNotUsable) {
		s.logger.Debugf("get stamp issuer: get issuer: %v", err)
		s.logger.Error("get stamp issuer: get issuer")
		jsonhttp.BadRequest(w, "cannot get issuer")
		return
	}

	exists, err := s.batchStore.Exists(id)
	if err != nil {
		s.logger.Debugf("get stamp issuer: check batch: %v", err)
		s.logger.Error("get stamp issuer: check batch")
		jsonhttp.InternalServerError(w, "unable to check batch")
		return
	}
	batchTTL, err := s.estimateBatchTTLFromID(id)
	if err != nil {
		s.logger.Debugf("get stamp issuer: estimate batch expiration: %v", err)
		s.logger.Error("get stamp issuer: estimate batch expiration")
		jsonhttp.InternalServerError(w, "unable to estimate batch expiration")
		return
	}

	resp := postageStampResponse{
		BatchID:  id,
		Exists:   exists,
		BatchTTL: batchTTL,
	}

	if issuer != nil {
		resp.Utilization = issuer.Utilization()
		resp.Usable = exists && s.post.IssuerUsable(issuer)
		resp.Label = issuer.Label()
		resp.Depth = issuer.Depth()
		resp.Amount = bigint.Wrap(issuer.Amount())
		resp.BucketDepth = issuer.BucketDepth()
		resp.BlockNumber = issuer.BlockNumber()
		resp.ImmutableFlag = issuer.ImmutableFlag()
	}

	jsonhttp.OK(w, &resp)
}

type reserveStateResponse struct {
	Radius        uint8          `json:"radius"`
	StorageRadius uint8          `json:"storageRadius"`
	Available     int64          `json:"available"`
	Commitment    int64          `json:"commitment"`
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

	commitment := int64(0)
	if err := s.batchStore.Iterate(func(b *postage.Batch) (bool, error) {
		commitment += int64(math.Pow(2.0, float64(b.Depth)))
		return false, nil
	}); err != nil {
		s.logger.Debugf("reserve state: batch store iteration: %v", err)
		s.logger.Error("reserve state: batch store iteration failed")

		jsonhttp.InternalServerError(w, "unable to iterate all batches")
		return
	}

	jsonhttp.OK(w, reserveStateResponse{
		Radius:        state.Radius,
		StorageRadius: state.StorageRadius,
		Available:     state.Available,
		Commitment:    int64(commitment),
		Outer:         bigint.Wrap(state.Outer),
		Inner:         bigint.Wrap(state.Inner),
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

// estimateBatchTTL estimates the time remaining until the batch expires.
// The -1 signals that the batch never expires.
func (s *Service) estimateBatchTTLFromID(id []byte) (int64, error) {
	batch, err := s.batchStore.Get(id)
	switch {
	case errors.Is(err, storage.ErrNotFound):
		return -1, nil
	case err != nil:
		return 0, err
	}

	return s.estimateBatchTTL(batch)
}

// estimateBatchTTL estimates the time remaining until the batch expires.
// The -1 signals that the batch never expires.
func (s *Service) estimateBatchTTL(batch *postage.Batch) (int64, error) {
	state := s.batchStore.GetChainState()
	if len(state.CurrentPrice.Bits()) == 0 {
		return -1, nil
	}

	var (
		normalizedBalance = batch.Value
		cumulativePayout  = state.TotalAmount
		pricePerBlock     = state.CurrentPrice
	)
	ttl := new(big.Int).Sub(normalizedBalance, cumulativePayout)
	ttl = ttl.Mul(ttl, s.blockTime)
	ttl = ttl.Div(ttl, pricePerBlock)

	return ttl.Int64(), nil
}

func (s *Service) postageTopUpHandler(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	if len(idStr) != 64 {
		s.logger.Error("topup batch: invalid batchID")
		jsonhttp.BadRequest(w, "invalid batchID")
		return
	}
	id, err := hex.DecodeString(idStr)
	if err != nil {
		s.logger.Debugf("topup batch: invalid batchID: %v", err)
		s.logger.Error("topup batch: invalid batchID")
		jsonhttp.BadRequest(w, "invalid batchID")
		return
	}

	amount, ok := big.NewInt(0).SetString(mux.Vars(r)["amount"], 10)
	if !ok {
		s.logger.Error("topup batch: invalid amount")
		jsonhttp.BadRequest(w, "invalid postage amount")
		return
	}

	ctx := r.Context()
	if price, ok := r.Header[gasPriceHeader]; ok {
		p, ok := big.NewInt(0).SetString(price[0], 10)
		if !ok {
			s.logger.Error("topup batch: bad gas price")
			jsonhttp.BadRequest(w, errBadGasPrice)
			return
		}
		ctx = sctx.SetGasPrice(ctx, p)
	}

	err = s.postageContract.TopUpBatch(ctx, id, amount)
	if err != nil {
		if errors.Is(err, postagecontract.ErrInsufficientFunds) {
			s.logger.Debugf("topup batch: out of funds: %v", err)
			s.logger.Error("topup batch: out of funds")
			jsonhttp.PaymentRequired(w, "out of funds")
			return
		}
		s.logger.Debugf("topup batch: failed to create: %v", err)
		s.logger.Error("topup batch: failed to create")
		jsonhttp.InternalServerError(w, "cannot topup batch")
		return
	}

	jsonhttp.Accepted(w, &postageCreateResponse{
		BatchID: id,
	})
}

func (s *Service) postageDiluteHandler(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	if len(idStr) != 64 {
		s.logger.Error("dilute batch: invalid batchID")
		jsonhttp.BadRequest(w, "invalid batchID")
		return
	}
	id, err := hex.DecodeString(idStr)
	if err != nil {
		s.logger.Debugf("dilute batch: invalid batchID: %v", err)
		s.logger.Error("dilute batch: invalid batchID")
		jsonhttp.BadRequest(w, "invalid batchID")
		return
	}

	depthStr := mux.Vars(r)["depth"]
	depth, err := strconv.ParseUint(depthStr, 10, 8)
	if err != nil {
		s.logger.Debugf("dilute batch: invalid depth: %v", err)
		s.logger.Error("dilute batch: invalid depth")
		jsonhttp.BadRequest(w, "invalid depth")
		return
	}

	ctx := r.Context()
	if price, ok := r.Header[gasPriceHeader]; ok {
		p, ok := big.NewInt(0).SetString(price[0], 10)
		if !ok {
			s.logger.Error("dilute batch: bad gas price")
			jsonhttp.BadRequest(w, errBadGasPrice)
			return
		}
		ctx = sctx.SetGasPrice(ctx, p)
	}

	err = s.postageContract.DiluteBatch(ctx, id, uint8(depth))
	if err != nil {
		if errors.Is(err, postagecontract.ErrInvalidDepth) {
			s.logger.Debugf("dilute batch: invalid depth: %v", err)
			s.logger.Error("dilte batch: invalid depth")
			jsonhttp.BadRequest(w, "invalid depth")
			return
		}
		s.logger.Debugf("dilute batch: failed to dilute: %v", err)
		s.logger.Error("dilute batch: failed to dilute")
		jsonhttp.InternalServerError(w, "cannot dilute batch")
		return
	}

	jsonhttp.Accepted(w, &postageCreateResponse{
		BatchID: id,
	})
}
