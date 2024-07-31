// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/ethersphere/bee/v2/pkg/bigint"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/postage/postagecontract"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/tracing"
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
			logger.Debug("create batch: no chain backend", "error", err)
			logger.Error(nil, "create batch: no chain backend")
			jsonhttp.MethodNotAllowed(w, "no chain backend")
			return
		}
		if errors.Is(err, postagecontract.ErrInsufficientFunds) {
			logger.Debug("create batch: out of funds", "error", err)
			logger.Error(nil, "create batch: out of funds")
			jsonhttp.BadRequest(w, "out of funds")
			return
		}
		if errors.Is(err, postagecontract.ErrInvalidDepth) {
			logger.Debug("create batch: invalid depth", "error", err)
			logger.Error(nil, "create batch: invalid depth")
			jsonhttp.BadRequest(w, "invalid depth")
			return
		}
		if errors.Is(err, postagecontract.ErrInsufficientValidity) {
			logger.Debug("create batch: insufficient validity", "error", err)
			logger.Error(nil, "create batch: insufficient validity")
			jsonhttp.BadRequest(w, "insufficient amount for 24h minimum validity")
			return
		}
		logger.Debug("create batch: create failed", "error", err)
		logger.Error(nil, "create batch: create failed")
		jsonhttp.InternalServerError(w, "cannot create batch")
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

	resp := postageStampsResponse{}
	stampIssuers := s.post.StampIssuers()
	resp.Stamps = make([]postageStampResponse, 0, len(stampIssuers))
	for _, v := range stampIssuers {

		exists, err := s.batchStore.Exists(v.ID())
		if err != nil {
			logger.Error(err, "get stamp issuer: check batch failed", "batch_id", hex.EncodeToString(v.ID()))
			jsonhttp.InternalServerError(w, "unable to check batch")
			return
		}
		if !exists {
			continue
		}

		batchTTL, err := s.estimateBatchTTLFromID(v.ID())
		if err != nil {
			logger.Error(err, "get stamp issuer: estimate batch expiration failed", "batch_id", hex.EncodeToString(v.ID()))
			jsonhttp.InternalServerError(w, "unable to estimate batch expiration")
			return
		}

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
			Exists:        true,
			BatchTTL:      batchTTL,
		})
	}

	jsonhttp.OK(w, resp)
}

func (s *Service) postageGetAllBatchesHandler(w http.ResponseWriter, _ *http.Request) {
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
		logger.Debug("iterate batches: iteration failed", "error", err)
		logger.Error(nil, "iterate batches: iteration failed")
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
		logger.Debug("get stamp issuer: get issuer failed", "batch_id", hexBatchID, "error", err)
		logger.Error(nil, "get stamp issuer: get issuer failed")
		switch {
		case errors.Is(err, postage.ErrNotUsable):
			jsonhttp.BadRequest(w, "batch not usable")
		case errors.Is(err, postage.ErrNotFound):
			jsonhttp.NotFound(w, "issuer does not exist")
		default:
			jsonhttp.InternalServerError(w, "get issuer failed")
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

	issuer, _, err := s.post.GetStampIssuer(paths.BatchID)
	if err != nil {
		logger.Debug("get stamp issuer: get issuer failed", "batch_id", hexBatchID, "error", err)
		logger.Error(nil, "get stamp issuer: get issuer failed")
		switch {
		case errors.Is(err, postage.ErrNotUsable):
			jsonhttp.BadRequest(w, "batch not usable")
		case errors.Is(err, postage.ErrNotFound):
			jsonhttp.NotFound(w, "issuer does not exist")
		default:
			jsonhttp.InternalServerError(w, "get issuer failed")
		}
		return
	}

	exists, err := s.batchStore.Exists(paths.BatchID)
	if err != nil {
		logger.Debug("exist check failed", "batch_id", hexBatchID, "error", err)
		logger.Error(nil, "exist check failed")
		jsonhttp.InternalServerError(w, "unable to check batch")
		return
	}
	if !exists {
		logger.Debug("batch does not exists", "batch_id", hexBatchID)
		jsonhttp.NotFound(w, "issuer does not exist")
		return
	}

	batchTTL, err := s.estimateBatchTTLFromID(paths.BatchID)
	if err != nil {
		logger.Debug("estimate batch expiration failed", "batch_id", hexBatchID, "error", err)
		logger.Error(nil, "estimate batch expiration failed")
		jsonhttp.InternalServerError(w, "unable to estimate batch expiration")
		return
	}

	jsonhttp.OK(w, &postageStampResponse{
		BatchID:       paths.BatchID,
		Depth:         issuer.Depth(),
		BucketDepth:   issuer.BucketDepth(),
		ImmutableFlag: issuer.ImmutableFlag(),
		Exists:        true,
		BatchTTL:      batchTTL,
		Utilization:   issuer.Utilization(),
		Usable:        s.post.IssuerUsable(issuer),
		Label:         issuer.Label(),
		Amount:        bigint.Wrap(issuer.Amount()),
		BlockNumber:   issuer.BlockNumber(),
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
		logger.Debug("batch store commitment calculation failed", "error", err)
		logger.Error(nil, "batch store commitment calculation failed")
		jsonhttp.InternalServerError(w, "unable to calculate commitment")
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
		logger.Debug("get block number failed", "error", err)
		logger.Error(nil, "get block number failed")
		jsonhttp.InternalServerError(w, "block number unavailable")
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
			logger.Debug("topup batch: out of funds", "batch_id", hexBatchID, "amount", paths.Amount, "error", err)
			logger.Error(nil, "topup batch: out of funds")
			jsonhttp.PaymentRequired(w, "out of funds")
			return
		}
		if errors.Is(err, postagecontract.ErrNotImplemented) {
			logger.Debug("topup batch: not implemented", "error", err)
			logger.Error(nil, "topup batch: not implemented")
			jsonhttp.NotImplemented(w, nil)
			return
		}
		logger.Debug("topup batch: topup failed", "batch_id", hexBatchID, "amount", paths.Amount, "error", err)
		logger.Error(nil, "topup batch: topup failed")
		jsonhttp.InternalServerError(w, "cannot topup batch")
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
			logger.Debug("dilute batch: invalid depth", "error", err)
			logger.Error(nil, "dilute batch: invalid depth")
			jsonhttp.BadRequest(w, "invalid depth")
			return
		}
		if errors.Is(err, postagecontract.ErrNotImplemented) {
			logger.Debug("dilute batch: not implemented", "error")
			logger.Error(nil, "dilute batch: not implemented")
			jsonhttp.NotImplemented(w, nil)
			return
		}
		logger.Debug("dilute batch: dilute failed", "batch_id", hexBatchID, "depth", paths.Depth, "error", err)
		logger.Error(nil, "dilute batch: dilute failed")
		jsonhttp.InternalServerError(w, "cannot dilute batch")
		return
	}

	jsonhttp.Accepted(w, &postageCreateResponse{
		BatchID: paths.BatchID,
		TxHash:  txHash.String(),
	})
}
