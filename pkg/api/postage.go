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
	"math/big"
	"net/http"
	"time"

	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/tracing"
	"github.com/gorilla/mux"
)

var defaultImmutable = true

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
			jsonhttp.ServiceUnavailable(w, "syncing failed")
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
	TxHash  string  `json:"txHash"`
}

func (s *Service) postageCreateHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("post_stamp").Build()

	paths := struct {
		Amount *big.Int `map:"amount" validate:"required"`
		Depth  uint8    `map:"depth" validate:"required,min=17"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	headers := struct {
		Immutable *bool `map:"Immutable"`
	}{}
	if response := s.mapStructure(r.Header, &headers); response != nil {
		response("invalid header params", logger, w)
		return
	}
	if headers.Immutable == nil {
		headers.Immutable = &defaultImmutable // Set the default.
	}

	txHash, batchID, err := s.postageContract.CreateBatch(
		r.Context(),
		paths.Amount,
		paths.Depth,
		*headers.Immutable,
		r.URL.Query().Get("label"),
	)
	if err != nil {
		if errors.Is(err, postagecontract.ErrChainDisabled) {
			logger.Debug("no chain backend", "error", err)
			logger.Warning("no chain backend")
			jsonhttp.MethodNotAllowed(w, "no chain backend")
			return
		}
		if errors.Is(err, postagecontract.ErrInsufficientFunds) {
			logger.Debug("out of funds", "amount", paths.Amount, "depth", paths.Depth, "error", err)
			logger.Warning("out of funds")
			jsonhttp.BadRequest(w, "out of funds")
			return
		}
		if errors.Is(err, postagecontract.ErrInvalidDepth) {
			logger.Debug("invalid depth", "amount", paths.Amount, "depth", paths.Depth, "error", err)
			logger.Warning("invalid depth")
			jsonhttp.BadRequest(w, "invalid depth")
			return
		}
		logger.Debug("unable to create batch", "amount", paths.Amount, "depth", paths.Depth, "error", err)
		logger.Error(nil, "unable to create batch")
		jsonhttp.InternalServerError(w, "create batch failed")
		return
	}

	jsonhttp.Created(w, &postageCreateResponse{
		BatchID: batchID,
		TxHash:  txHash.String(),
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
	BatchID     hexByte        `json:"batchID"`
	Value       *bigint.BigInt `json:"value"`
	Start       uint64         `json:"start"`
	Owner       hexByte        `json:"owner"`
	Depth       uint8          `json:"depth"`
	BucketDepth uint8          `json:"bucketDepth"`
	Immutable   bool           `json:"immutable"`
	BatchTTL    int64          `json:"batchTTL"`
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
	logger := s.logger.WithName("get_stamps").Build()

	queries := struct {
		All bool `map:"all"`
	}{}
	if response := s.mapStructure(r.URL.Query(), &queries); response != nil {
		response("invalid query params", logger, w)
		return
	}

	resp := postageStampsResponse{}
	stampIssuers := s.post.StampIssuers()
	resp.Stamps = make([]postageStampResponse, 0, len(stampIssuers))
	for _, v := range stampIssuers {
		exists, err := s.batchStore.Exists(v.ID())
		if err != nil {
			logger.Debug("unable to check batch", "batch_id", hex.EncodeToString(v.ID()), "error", err)
			logger.Warning("unable to check batch")
			jsonhttp.InternalServerError(w, "check batch failed")
			return
		}

		batchTTL, err := s.estimateBatchTTLFromID(v.ID())
		if err != nil {
			logger.Debug("unable to estimate batch expiration", "batch_id", hex.EncodeToString(v.ID()), "error", err)
			logger.Warning("unable to estimate batch expiration")
			jsonhttp.InternalServerError(w, "estimate batch expiration failed")
			return
		}
		if queries.All || exists {
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
	logger := s.logger.WithName("get_batches").Build()

	batches := make([]postageBatchResponse, 0)
	err := s.batchStore.Iterate(func(b *postage.Batch) (bool, error) {
		batchTTL, err := s.estimateBatchTTL(b)
		if err != nil {
			return false, fmt.Errorf("estimate batch ttl: %w", err)
		}

		batches = append(batches, postageBatchResponse{
			BatchID:     b.ID,
			Value:       bigint.Wrap(b.Value),
			Start:       b.Start,
			Owner:       b.Owner,
			Depth:       b.Depth,
			BucketDepth: b.BucketDepth,
			Immutable:   b.Immutable,
			BatchTTL:    batchTTL,
		})
		return false, nil
	})
	if err != nil {
		logger.Debug("unable to iteration batches", "error", err)
		logger.Warning("unable to iteration batches")
		jsonhttp.InternalServerError(w, "iterate batches failed")
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
	logger := s.logger.WithName("get_stamp_bucket").Build()

	paths := struct {
		BatchID []byte `map:"batch_id" validate:"required,len=32"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}
	hexBatchID := hex.EncodeToString(paths.BatchID)

	issuer, _, err := s.post.GetStampIssuer(paths.BatchID)
	if err != nil {
		logger.Debug("unable to get stamp issuer", "batch_id", hexBatchID, "error", err)
		logger.Warning("unable to get stamp issuer")
		switch {
		case errors.Is(err, postage.ErrNotUsable):
			jsonhttp.BadRequest(w, "batch not usable")
		case errors.Is(err, postage.ErrNotFound):
			jsonhttp.NotFound(w, "batch not found")
		default:
			jsonhttp.InternalServerError(w, "get stamp issuer failed")
		}
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
	logger := s.logger.WithName("get_stamp").Build()

	paths := struct {
		BatchID []byte `map:"batch_id" validate:"required,len=32"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}
	hexBatchID := hex.EncodeToString(paths.BatchID)

	var issuer *postage.StampIssuer
	stampIssuers := s.post.StampIssuers()
	for _, stampIssuer := range stampIssuers {
		if bytes.Equal(paths.BatchID, stampIssuer.ID()) {
			issuer = stampIssuer
			break
		}
	}
	if issuer == nil {
		logger.Debug("unable to get stamp issuer", "batch_id", hexBatchID)
		logger.Warning("unable to get stamp issuer")
		jsonhttp.BadRequest(w, "get stamp issuer failed")
		return
	}

	exists, err := s.batchStore.Exists(paths.BatchID)
	if err != nil {
		logger.Debug("unable to check batch", "batch_id", hexBatchID, "error", err)
		logger.Warning("unable to check batch")
		jsonhttp.InternalServerError(w, "check batch failed")
		return
	}
	batchTTL, err := s.estimateBatchTTLFromID(paths.BatchID)
	if err != nil {
		logger.Debug("unable to estimate batch expiration", "batch_id", hexBatchID, "error", err)
		logger.Warning("unable to estimate batch expiration")
		jsonhttp.InternalServerError(w, "estimate batch expiration failed")
		return
	}

	jsonhttp.OK(w, &postageStampResponse{
		BatchID:       paths.BatchID,
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
	Radius        uint8  `json:"radius"`
	StorageRadius uint8  `json:"storageRadius"`
	Commitment    uint64 `json:"commitment"`
}

type chainStateResponse struct {
	ChainTip     uint64         `json:"chainTip"`     // ChainTip (block height).
	Block        uint64         `json:"block"`        // The block number of the last postage event.
	TotalAmount  *bigint.BigInt `json:"totalAmount"`  // Cumulative amount paid per stamp.
	CurrentPrice *bigint.BigInt `json:"currentPrice"` // Bzz/chunk/block normalised price.
}

func (s *Service) reserveStateHandler(w http.ResponseWriter, _ *http.Request) {
	logger := s.logger.WithName("get_reservestate").Build()

	commitment, err := s.batchStore.Commitment()
	if err != nil {
		logger.Debug("unable to calculate batch store commitment", "error", err)
		logger.Warning("unable to calculate batch store commitment")
		jsonhttp.InternalServerError(w, "batch store commitment calculation failed")
		return
	}

	jsonhttp.OK(w, reserveStateResponse{
		Radius:        s.batchStore.Radius(),
		StorageRadius: s.storer.StorageRadius(),
		Commitment:    commitment,
	})
}

// chainStateHandler returns the current chain state.
func (s *Service) chainStateHandler(w http.ResponseWriter, r *http.Request) {
	logger := tracing.NewLoggerWithTraceID(r.Context(), s.logger.WithName("get_chainstate").Build())

	state := s.batchStore.GetChainState()
	chainTip, err := s.chainBackend.BlockNumber(r.Context())
	if err != nil {
		logger.Debug("unable to get block number", "error", err)
		logger.Warning("unable to get block number")
		jsonhttp.InternalServerError(w, "get block number failed")
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
	ttl = ttl.Mul(ttl, big.NewInt(int64(s.blockTime/time.Second)))
	ttl = ttl.Div(ttl, pricePerBlock)

	return ttl.Int64(), nil
}

func (s *Service) postageTopUpHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("patch_stamp_topup").Build()

	paths := struct {
		BatchID []byte   `map:"batch_id" validate:"required,len=32"`
		Amount  *big.Int `map:"amount" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}
	hexBatchID := hex.EncodeToString(paths.BatchID)

	txHash, err := s.postageContract.TopUpBatch(r.Context(), paths.BatchID, paths.Amount)
	if err != nil {
		if errors.Is(err, postagecontract.ErrInsufficientFunds) {
			logger.Debug("out of funds", "batch_id", hexBatchID, "amount", paths.Amount, "error", err)
			logger.Warning("out of funds")
			jsonhttp.PaymentRequired(w, "out of funds")
			return
		}
		if errors.Is(err, postagecontract.ErrNotImplemented) {
			logger.Debug("not implemented", "error", err)
			logger.Warning("not implemented")
			jsonhttp.NotImplemented(w, nil)
			return
		}
		logger.Debug("topup failed", "batch_id", hexBatchID, "amount", paths.Amount, "error", err)
		logger.Warning("topup failed")
		jsonhttp.InternalServerError(w, "topup batch failed")
		return
	}

	jsonhttp.Accepted(w, &postageCreateResponse{
		BatchID: paths.BatchID,
		TxHash:  txHash.String(),
	})
}

func (s *Service) postageDiluteHandler(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("patch_stamp_dilute").Build()

	paths := struct {
		BatchID []byte `map:"batch_id" validate:"required,len=32"`
		Depth   uint8  `map:"depth" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}
	hexBatchID := hex.EncodeToString(paths.BatchID)

	txHash, err := s.postageContract.DiluteBatch(r.Context(), paths.BatchID, paths.Depth)
	if err != nil {
		if errors.Is(err, postagecontract.ErrInvalidDepth) {
			logger.Debug("unable to dilute batch", "batch_id", hexBatchID, "depth", paths.Depth, "error", err)
			logger.Warning("unable to dilute batch")
			jsonhttp.BadRequest(w, "invalid depth")
			return
		}
		if errors.Is(err, postagecontract.ErrNotImplemented) {
			logger.Debug("not implemented", "error")
			logger.Warning("not implemented")
			jsonhttp.NotImplemented(w, nil)
			return
		}
		logger.Debug("unable to dilute batch", "batch_id", hexBatchID, "depth", paths.Depth, "error", err)
		logger.Warning("unable to dilute batch")
		jsonhttp.InternalServerError(w, "dilute batch failed")
		return
	}

	jsonhttp.Accepted(w, &postageCreateResponse{
		BatchID: paths.BatchID,
		TxHash:  txHash.String(),
	})
}
