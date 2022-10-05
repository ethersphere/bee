// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/gorilla/mux"
)

func (s *Service) postageAccessHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.postageSem.TryAcquire(1) {
			s.logger.Debug("postage access: simultaneous on-chain operations not supported")
			s.logger.Error(nil, "postage access: simultaneous on-chain operations not supported")
			jsonhttp.TooManyRequests(w, "simultaneous on-chain operations not supported")
			return
		}
		defer s.postageSem.Release(1)

		h.ServeHTTP(w, r)
	})
}

func (s *Service) postageSyncStatusCheckHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		done, err := s.syncStatus()
		if err != nil {
			s.logger.Debug("postage access: syncing failed", "error", err)
			s.logger.Error(nil, "postage access: syncing failed")
			jsonhttp.ServiceUnavailable(w, "postage: syncing failed")
			return
		}
		if !done {
			s.logger.Debug("postage access: syncing in progress")
			s.logger.Error(nil, "postage access: syncing in progress")
			jsonhttp.ServiceUnavailable(w, "syncing in progress")
			return
		}

		h.ServeHTTP(w, r)
	})
}

// hexByte takes care that a byte slice gets correctly
// marshaled by the json serializer.
type hexByte []byte

func (b hexByte) MarshalJSON() ([]byte, error) {
	return json.Marshal(hex.EncodeToString(b))
}

type postageCreateResponse struct {
	BatchID hexByte `json:"batchID"`
}

func (s *Service) postageCreateHandler(w http.ResponseWriter, r *http.Request) {
	depthStr := mux.Vars(r)["depth"]

	amount, ok := big.NewInt(0).SetString(mux.Vars(r)["amount"], 10)
	if !ok {
		s.logger.Error(nil, "create batch: invalid amount")
		jsonhttp.BadRequest(w, "invalid postage amount")
		return

	}

	depth, err := strconv.ParseUint(depthStr, 10, 8)
	if err != nil {
		s.logger.Debug("create batch: parse depth string failed", "string", depthStr, "error", err)
		s.logger.Error(nil, "create batch: parse depth string failed")
		jsonhttp.BadRequest(w, "invalid depth")
		return
	}

	label := r.URL.Query().Get("label")

	var immutable bool
	if val, ok := r.Header[immutableHeader]; ok {
		immutable, _ = strconv.ParseBool(val[0])
	}

	batchID, err := s.postageContract.CreateBatch(r.Context(), amount, uint8(depth), immutable, label)
	if err != nil {
		if errors.Is(err, postagecontract.ErrChainDisabled) {
			s.logger.Debug("create batch: no chain backend", "error", err)
			s.logger.Error(nil, "create batch: no chain backend")
			jsonhttp.MethodNotAllowed(w, "no chain backend")
			return
		}
		if errors.Is(err, postagecontract.ErrInsufficientFunds) {
			s.logger.Debug("create batch: out of funds", "error", err)
			s.logger.Error(nil, "create batch: out of funds")
			jsonhttp.BadRequest(w, "out of funds")
			return
		}
		if errors.Is(err, postagecontract.ErrInvalidDepth) {
			s.logger.Debug("create batch: invalid depth", "error", err)
			s.logger.Error(nil, "create batch: invalid depth")
			jsonhttp.BadRequest(w, "invalid depth")
			return
		}
		s.logger.Debug("create batch: create failed", "error", err)
		s.logger.Error(nil, "create batch: create failed")
		jsonhttp.InternalServerError(w, "cannot create batch")
		return
	}

	jsonhttp.Created(w, &postageCreateResponse{
		BatchID: batchID,
	})
}

type postageStampResponse struct {
	BatchID       hexByte        `json:"batchID"`
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
	Expired       bool           `json:"expired"`
}

type postageStampsResponse struct {
	Stamps []postageStampResponse `json:"stamps"`
}

type postageBatchResponse struct {
	BatchID       hexByte        `json:"batchID"`
	Value         *bigint.BigInt `json:"value"`
	Start         uint64         `json:"start"`
	Owner         hexByte        `json:"owner"`
	Depth         uint8          `json:"depth"`
	BucketDepth   uint8          `json:"bucketDepth"`
	Immutable     bool           `json:"immutable"`
	StorageRadius uint8          `json:"storageRadius"`
	BatchTTL      int64          `json:"batchTTL"`
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
	isAll := strings.ToLower(r.URL.Query().Get("all")) == "true"
	resp := postageStampsResponse{}
	resp.Stamps = make([]postageStampResponse, 0, len(s.post.StampIssuers()))
	for _, v := range s.post.StampIssuers() {
		exists, err := s.batchStore.Exists(v.ID())
		if err != nil {
			s.logger.Debug("get stamp issuer: check batch failed", "batch_id", hex.EncodeToString(v.ID()), "error", err)
			s.logger.Error(nil, "get stamp issuer: check batch failed")
			jsonhttp.InternalServerError(w, "unable to check batch")
			return
		}

		batchTTL, err := s.estimateBatchTTLFromID(v.ID())
		if err != nil {
			s.logger.Debug("get stamp issuer: estimate batch expiration failed", "batch_id", hex.EncodeToString(v.ID()), "error", err)
			s.logger.Error(nil, "get stamp issuer: estimate batch expiration failed")
			jsonhttp.InternalServerError(w, "unable to estimate batch expiration")
			return
		}
		if isAll || exists {
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
				Expired:       v.Expired(),
			})
		}
	}

	jsonhttp.OK(w, resp)
}

func (s *Service) postageGetAllStampsHandler(w http.ResponseWriter, _ *http.Request) {
	batches := make([]postageBatchResponse, 0)
	err := s.batchStore.Iterate(func(b *postage.Batch) (bool, error) {
		batchTTL, err := s.estimateBatchTTL(b)
		if err != nil {
			return false, fmt.Errorf("estimate batch ttl: %w", err)
		}

		batches = append(batches, postageBatchResponse{
			BatchID:       b.ID,
			Value:         bigint.Wrap(b.Value),
			Start:         b.Start,
			Owner:         b.Owner,
			Depth:         b.Depth,
			BucketDepth:   b.BucketDepth,
			Immutable:     b.Immutable,
			StorageRadius: b.StorageRadius,
			BatchTTL:      batchTTL,
		})
		return false, nil
	})
	if err != nil {
		s.logger.Debug("iterate batches: iteration failed", "error", err)
		s.logger.Error(nil, "iterate batches: iteration failed")
		jsonhttp.InternalServerError(w, "unable to iterate all batches")
		return
	}

	batchesRes := struct {
		Batches []postageBatchResponse `json:"batches"`
	}{
		Batches: batches,
	}

	jsonhttp.OK(w, batchesRes)
}

func (s *Service) postageGetStampBucketsHandler(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	if len(idStr) != 64 {
		s.logger.Error(nil, "get stamp issuer: invalid batch id string length", "string", idStr, "length", len(idStr))
		jsonhttp.BadRequest(w, "invalid batchID")
		return
	}
	id, err := hex.DecodeString(idStr)
	if err != nil {
		s.logger.Debug("get stamp issuer: decode batch id string failed", "string", idStr, "error", err)
		s.logger.Error(nil, "get stamp issuer: decode batch id string failed")
		jsonhttp.BadRequest(w, "invalid batchID")
		return
	}

	issuer, err := s.post.GetStampIssuer(id)
	if err != nil {
		s.logger.Debug("get stamp issuer: get issuer failed", "batch_id", hex.EncodeToString(id), "error", err)
		s.logger.Error(nil, "get stamp issuer: get issuer failed")
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
	logger := s.logger.WithName("get_stamp_by_id").Build()
	idStr := mux.Vars(r)["id"]
	if len(idStr) != 64 {
		s.logger.Error(nil, "get stamp issuer: invalid batch id string length", "string", idStr, "length", len(idStr))
		jsonhttp.BadRequest(w, "invalid batchID")
		return
	}
	id, err := hex.DecodeString(idStr)
	if err != nil {
		logger.Debug("decode batch id string failed", "string", idStr, "error", err)
		logger.Error(nil, "decode batch id string failed")
		jsonhttp.BadRequest(w, "invalid batchID")
		return
	}
	var issuer *postage.StampIssuer
	for _, stampIssuer := range s.post.StampIssuers() {
		if bytes.Equal(id, stampIssuer.ID()) {
			issuer = stampIssuer
			break
		}
	}
	if issuer == nil {
		s.logger.Debug("get issuer failed", "batch_id", hex.EncodeToString(id))
		s.logger.Error(nil, "get issuer failed")
		jsonhttp.BadRequest(w, "cannot find batch")
		return
	}

	exists, err := s.batchStore.Exists(id)
	if err != nil {
		s.logger.Debug("get stamp issuer: exist check failed", "batch_id", hex.EncodeToString(id), "error", err)
		s.logger.Error(nil, "get stamp issuer: exist check failed")
		jsonhttp.InternalServerError(w, "unable to check batch")
		return
	}
	batchTTL, err := s.estimateBatchTTLFromID(id)
	if err != nil {
		s.logger.Debug("get stamp issuer: estimate batch expiration failed", "batch_id", hex.EncodeToString(id), "error", err)
		s.logger.Error(nil, "get stamp issuer: estimate batch expiration failed")
		jsonhttp.InternalServerError(w, "unable to estimate batch expiration")
		return
	}

	jsonhttp.OK(w, &postageStampResponse{
		BatchID:       id,
		Depth:         issuer.Depth(),
		BucketDepth:   issuer.BucketDepth(),
		ImmutableFlag: issuer.ImmutableFlag(),
		Exists:        exists,
		BatchTTL:      batchTTL,
		Utilization:   issuer.Utilization(),
		Usable:        exists && s.post.IssuerUsable(issuer),
		Label:         issuer.Label(),
		Amount:        bigint.Wrap(issuer.Amount()),
		BlockNumber:   issuer.BlockNumber(),
		Expired:       issuer.Expired(),
	})
}

type reserveStateResponse struct {
	Radius        uint8 `json:"radius"`
	StorageRadius uint8 `json:"storageRadius"`
	Commitment    int64 `json:"commitment"`
}

type chainStateResponse struct {
	ChainTip     uint64         `json:"chainTip"`     // ChainTip (block height).
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
		s.logger.Debug("reserve state: batch store iteration failed", "error", err)
		s.logger.Error(nil, "reserve state: batch store iteration failed")

		jsonhttp.InternalServerError(w, "unable to iterate all batches")
		return
	}

	jsonhttp.OK(w, reserveStateResponse{
		Radius:        state.Radius,
		StorageRadius: state.StorageRadius,
		Commitment:    commitment,
	})
}

// chainStateHandler returns the current chain state.
func (s *Service) chainStateHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger)
	state := s.batchStore.GetChainState()
	chainTip, err := s.chainBackend.BlockNumber(r.Context())
	if err != nil {
		logger.Debug("chainstate: get block number failed", "error", err)
		logger.Error(nil, "chainstate: get block number failed")
		jsonhttp.InternalServerError(w, "chainstate: block number unavailable")
		return
	}
	jsonhttp.OK(w, chainStateResponse{
		ChainTip:     chainTip,
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
		s.logger.Error(nil, "topup batch: invalid batch id string length", "string", idStr, "length", len(idStr))
		jsonhttp.BadRequest(w, "invalid batchID")
		return
	}
	id, err := hex.DecodeString(idStr)
	if err != nil {
		s.logger.Debug("topup batch: decode batch id string failed", "string", idStr, "error", err)
		s.logger.Error(nil, "topup batch: decode batch id string failed")
		jsonhttp.BadRequest(w, "invalid batchID")
		return
	}

	amountStr := mux.Vars(r)["amount"]
	amount, ok := big.NewInt(0).SetString(amountStr, 10)
	if !ok {
		s.logger.Error(nil, "topup batch: parese amount string failed", "string", amountStr)
		jsonhttp.BadRequest(w, "invalid postage amount")
		return
	}

	err = s.postageContract.TopUpBatch(r.Context(), id, amount)
	if err != nil {
		if errors.Is(err, postagecontract.ErrInsufficientFunds) {
			s.logger.Debug("topup batch: out of funds", "batch_id", hex.EncodeToString(id), "amount", amount, "error", err)
			s.logger.Error(nil, "topup batch: out of funds")
			jsonhttp.PaymentRequired(w, "out of funds")
			return
		}
		if errors.Is(err, postagecontract.ErrNotImplemented) {
			s.logger.Debug("topup batch: not implemented", "error", err)
			s.logger.Error(nil, "topup batch: not implemented")
			jsonhttp.NotImplemented(w, nil)
			return
		}
		s.logger.Debug("topup batch: topup failed", "batch_id", hex.EncodeToString(id), "amount", amount, "error", err)
		s.logger.Error(nil, "topup batch: topup failed")
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
		s.logger.Error(nil, "dilute batch: invalid batch id string length", "string", idStr, "length", len(idStr))
		jsonhttp.BadRequest(w, "invalid batchID")
		return
	}
	id, err := hex.DecodeString(idStr)
	if err != nil {
		s.logger.Debug("dilute batch: decode batch id string failed", "string", idStr, "error", err)
		s.logger.Error(nil, "dilute batch: decode batch id string failed")
		jsonhttp.BadRequest(w, "invalid batchID")
		return
	}

	depthStr := mux.Vars(r)["depth"]
	depth, err := strconv.ParseUint(depthStr, 10, 8)
	if err != nil {
		s.logger.Debug("dilute batch: parse depth string failed", "string", depthStr, "error", err)
		s.logger.Error(nil, "dilute batch: parse depth string failed")
		jsonhttp.BadRequest(w, "invalid depth")
		return
	}

	err = s.postageContract.DiluteBatch(r.Context(), id, uint8(depth))
	if err != nil {
		if errors.Is(err, postagecontract.ErrInvalidDepth) {
			s.logger.Debug("dilute batch: invalid depth", "error", err)
			s.logger.Error(nil, "dilute batch: invalid depth")
			jsonhttp.BadRequest(w, "invalid depth")
			return
		}
		if errors.Is(err, postagecontract.ErrNotImplemented) {
			s.logger.Debug("dilute batch: not implemented", "error", err)
			s.logger.Error(nil, "dilute batch: not implemented")
			jsonhttp.NotImplemented(w, nil)
			return
		}
		s.logger.Debug("dilute batch: dilute failed", "batch_id", hex.EncodeToString(id), "depth", depth, "error", err)
		s.logger.Error(nil, "dilute batch: dilute failed")
		jsonhttp.InternalServerError(w, "cannot dilute batch")
		return
	}

	jsonhttp.Accepted(w, &postageCreateResponse{
		BatchID: id,
	})
}
