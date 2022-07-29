// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	mockpost "github.com/ethersphere/bee/pkg/postage/mock"
	"github.com/ethersphere/bee/pkg/postage/postagecontract"
	contractMock "github.com/ethersphere/bee/pkg/postage/postagecontract/mock"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
	"github.com/ethersphere/bee/pkg/sctx"
)

func TestPostageCreateStamp(t *testing.T) {
	batchID := []byte{1, 2, 3, 4}
	initialBalance := int64(1000)
	depth := uint8(1)
	label := "label"
	createBatch := func(amount int64, depth uint8, label string) string {
		return fmt.Sprintf("/stamps/%d/%d?label=%s", amount, depth, label)
	}

	t.Run("ok", func(t *testing.T) {
		contract := contractMock.New(
			contractMock.WithCreateBatchFunc(func(ctx context.Context, ib *big.Int, d uint8, i bool, l string) ([]byte, error) {
				if ib.Cmp(big.NewInt(initialBalance)) != 0 {
					return nil, fmt.Errorf("called with wrong initial balance. wanted %d, got %d", initialBalance, ib)
				}
				if d != depth {
					return nil, fmt.Errorf("called with wrong depth. wanted %d, got %d", depth, d)
				}
				if l != label {
					return nil, fmt.Errorf("called with wrong label. wanted %s, got %s", label, l)
				}
				return batchID, nil
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:        true,
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPost, createBatch(initialBalance, depth, label), http.StatusCreated,
			jsonhttptest.WithExpectedJSONResponse(&api.PostageCreateResponse{
				BatchID: batchID,
			}),
		)
	})
	t.Run("with-custom-gas", func(t *testing.T) {
		contract := contractMock.New(
			contractMock.WithCreateBatchFunc(func(ctx context.Context, ib *big.Int, d uint8, i bool, l string) ([]byte, error) {
				if ib.Cmp(big.NewInt(initialBalance)) != 0 {
					return nil, fmt.Errorf("called with wrong initial balance. wanted %d, got %d", initialBalance, ib)
				}
				if d != depth {
					return nil, fmt.Errorf("called with wrong depth. wanted %d, got %d", depth, d)
				}
				if l != label {
					return nil, fmt.Errorf("called with wrong label. wanted %s, got %s", label, l)
				}
				if sctx.GetGasPrice(ctx).Cmp(big.NewInt(10000)) != 0 {
					return nil, fmt.Errorf("called with wrong gas price. wanted %d, got %d", 10000, sctx.GetGasPrice(ctx))
				}
				return batchID, nil
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:        true,
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPost, createBatch(initialBalance, depth, label), http.StatusCreated,
			jsonhttptest.WithRequestHeader("Gas-Price", "10000"),
			jsonhttptest.WithExpectedJSONResponse(&api.PostageCreateResponse{
				BatchID: batchID,
			}),
		)
	})

	t.Run("with-error", func(t *testing.T) {
		contract := contractMock.New(
			contractMock.WithCreateBatchFunc(func(ctx context.Context, ib *big.Int, d uint8, i bool, l string) ([]byte, error) {
				return nil, errors.New("err")
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:        true,
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPost, createBatch(initialBalance, depth, label), http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusInternalServerError,
				Message: "cannot create batch",
			}),
		)
	})

	t.Run("out-of-funds", func(t *testing.T) {
		contract := contractMock.New(
			contractMock.WithCreateBatchFunc(func(ctx context.Context, ib *big.Int, d uint8, i bool, l string) ([]byte, error) {
				return nil, postagecontract.ErrInsufficientFunds
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:        true,
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPost, createBatch(initialBalance, depth, label), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "out of funds",
			}),
		)
	})

	t.Run("invalid depth", func(t *testing.T) {
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true})

		jsonhttptest.Request(t, ts, http.MethodPost, "/stamps/1000/ab", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid depth",
			}),
		)
	})

	t.Run("depth less than bucket depth", func(t *testing.T) {
		contract := contractMock.New(
			contractMock.WithCreateBatchFunc(func(ctx context.Context, ib *big.Int, d uint8, i bool, l string) ([]byte, error) {
				return nil, postagecontract.ErrInvalidDepth
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:        true,
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPost, "/stamps/1000/9", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid depth",
			}),
		)
	})

	t.Run("invalid balance", func(t *testing.T) {
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true})

		jsonhttptest.Request(t, ts, http.MethodPost, "/stamps/abcd/2", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid postage amount",
			}),
		)
	})

	t.Run("immutable header", func(t *testing.T) {

		var immutable bool
		contract := contractMock.New(
			contractMock.WithCreateBatchFunc(func(ctx context.Context, _ *big.Int, _ uint8, i bool, _ string) ([]byte, error) {
				immutable = i
				return batchID, nil
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:        true,
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPost, "/stamps/1000/24", http.StatusCreated,
			jsonhttptest.WithRequestHeader("Immutable", "true"),
			jsonhttptest.WithExpectedJSONResponse(&api.PostageCreateResponse{
				BatchID: batchID,
			}),
		)

		if !immutable {
			t.Fatalf("want true, got %v", immutable)
		}
	})

	t.Run("syncing in progress", func(t *testing.T) {
		ts, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:          true,
			BatchEventUpdater: mock.NewNotReady(),
		})

		jsonhttptest.Request(t, ts, http.MethodPost, createBatch(initialBalance, depth, label), http.StatusServiceUnavailable,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Message: "syncing in progress",
				Code:    503,
			}),
		)
	})
	t.Run("syncing failed", func(t *testing.T) {
		ts, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:          true,
			BatchEventUpdater: mock.NewWithError(errors.New("oops")),
		})

		jsonhttptest.Request(t, ts, http.MethodPost, createBatch(initialBalance, depth, label), http.StatusServiceUnavailable,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Message: "batch unavailable: syncing failed",
				Code:    503,
			}),
		)
	})
}

func TestPostageGetStamps(t *testing.T) {
	b := postagetesting.MustNewBatch()
	b.Value = big.NewInt(20)
	si := postage.NewStampIssuer("", "", b.ID, big.NewInt(3), 11, 10, 1000, true)
	mp := mockpost.New(mockpost.WithIssuer(si))
	cs := &postage.ChainState{Block: 10, TotalAmount: big.NewInt(5), CurrentPrice: big.NewInt(2)}
	bs := mock.New(mock.WithChainState(cs), mock.WithBatch(b))
	ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true, Post: mp, BatchStore: bs, BlockTime: big.NewInt(2)})

	t.Run("single stamp", func(t *testing.T) {
		jsonhttptest.Request(t, ts, http.MethodGet, "/stamps", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&api.PostageStampsResponse{
				Stamps: []api.PostageStampResponse{
					{
						BatchID:       b.ID,
						Utilization:   si.Utilization(),
						Usable:        true,
						Label:         si.Label(),
						Depth:         si.Depth(),
						Amount:        bigint.Wrap(si.Amount()),
						BucketDepth:   si.BucketDepth(),
						BlockNumber:   si.BlockNumber(),
						ImmutableFlag: si.ImmutableFlag(),
						Exists:        true,
						BatchTTL:      15, // ((value-totalAmount)/pricePerBlock)*blockTime=((20-5)/2)*2.
					},
				},
			}),
		)
	})
}

// TestGetAllBatches tests that the endpoint that returns all living
// batches functions correctly.
func TestGetAllBatches(t *testing.T) {
	b := postagetesting.MustNewBatch()
	b.Value = big.NewInt(20)
	si := postage.NewStampIssuer("", "", b.ID, big.NewInt(3), 11, 10, 1000, true)
	mp := mockpost.New(mockpost.WithIssuer(si))
	cs := &postage.ChainState{Block: 10, TotalAmount: big.NewInt(5), CurrentPrice: big.NewInt(2)}
	bs := mock.New(mock.WithChainState(cs), mock.WithBatch(b))
	ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true, Post: mp, BatchStore: bs, BlockTime: big.NewInt(2)})

	oneBatch := struct {
		Batches []api.PostageBatchResponse `json:"batches"`
	}{
		Batches: []api.PostageBatchResponse{
			{
				BatchID:       b.ID,
				Value:         bigint.Wrap(b.Value),
				Start:         b.Start,
				Owner:         b.Owner,
				Depth:         b.Depth,
				BucketDepth:   b.BucketDepth,
				Immutable:     b.Immutable,
				StorageRadius: b.StorageRadius,
				BatchTTL:      15, // ((value-totalAmount)/pricePerBlock)*blockTime=((20-5)/2)*2.
			},
		},
	}

	t.Run("all stamps", func(t *testing.T) {
		jsonhttptest.Request(t, ts, http.MethodGet, "/batches", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(oneBatch),
		)
	})
}

func TestPostageGetStamp(t *testing.T) {
	b := postagetesting.MustNewBatch()
	b.Value = big.NewInt(20)
	si := postage.NewStampIssuer("", "", b.ID, big.NewInt(3), 11, 10, 1000, true)
	mp := mockpost.New(mockpost.WithIssuer(si))
	cs := &postage.ChainState{Block: 10, TotalAmount: big.NewInt(5), CurrentPrice: big.NewInt(2)}
	bs := mock.New(mock.WithChainState(cs), mock.WithBatch(b))
	ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true, Post: mp, BatchStore: bs, BlockTime: big.NewInt(2)})

	t.Run("ok", func(t *testing.T) {
		jsonhttptest.Request(t, ts, http.MethodGet, "/stamps/"+hex.EncodeToString(b.ID), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&api.PostageStampResponse{
				BatchID:       b.ID,
				Utilization:   si.Utilization(),
				Usable:        true,
				Label:         si.Label(),
				Depth:         si.Depth(),
				Amount:        bigint.Wrap(si.Amount()),
				BucketDepth:   si.BucketDepth(),
				BlockNumber:   si.BlockNumber(),
				ImmutableFlag: si.ImmutableFlag(),
				Exists:        true,
				BatchTTL:      15, // ((value-totalAmount)/pricePerBlock)*blockTime=((20-5)/2)*2.
			}),
		)
	})
	t.Run("bad request", func(t *testing.T) {
		badBatch := []byte{0, 1, 2}

		jsonhttptest.Request(t, ts, http.MethodGet, "/stamps/"+hex.EncodeToString(badBatch), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid batchID",
			}),
		)
	})
	t.Run("bad request", func(t *testing.T) {
		badBatch := []byte{0, 1, 2, 4}

		jsonhttptest.Request(t, ts, http.MethodGet, "/stamps/"+hex.EncodeToString(badBatch), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid batchID",
			}),
		)
	})
}

func TestPostageGetBuckets(t *testing.T) {
	si := postage.NewStampIssuer("", "", batchOk, big.NewInt(3), 11, 10, 1000, true)
	mp := mockpost.New(mockpost.WithIssuer(si))
	ts, _, _, _ := newTestServer(t, testServerOptions{Post: mp, DebugAPI: true})
	buckets := make([]api.BucketData, 1024)
	for i := range buckets {
		buckets[i] = api.BucketData{BucketID: uint32(i)}
	}

	t.Run("ok", func(t *testing.T) {
		jsonhttptest.Request(t, ts, http.MethodGet, "/stamps/"+batchOkStr+"/buckets", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&api.PostageStampBucketsResponse{
				Depth:            si.Depth(),
				BucketDepth:      si.BucketDepth(),
				BucketUpperBound: si.BucketUpperBound(),
				Buckets:          buckets,
			}),
		)
	})
	t.Run("bad batch", func(t *testing.T) {
		badBatch := []byte{0, 1, 2}

		jsonhttptest.Request(t, ts, http.MethodGet, "/stamps/"+hex.EncodeToString(badBatch)+"/buckets", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid batchID",
			}),
		)
	})
	t.Run("bad batch", func(t *testing.T) {
		badBatch := []byte{0, 1, 2, 4}

		jsonhttptest.Request(t, ts, http.MethodGet, "/stamps/"+hex.EncodeToString(badBatch)+"/buckets", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid batchID",
			}),
		)
	})
}

func TestReserveState(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		ts, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI: true,
			BatchStore: mock.New(mock.WithReserveState(&postage.ReserveState{
				Radius: 5,
			})),
		})
		jsonhttptest.Request(t, ts, http.MethodGet, "/reservestate", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&api.ReserveStateResponse{
				Radius: 5,
			}),
		)
	})
	t.Run("empty", func(t *testing.T) {
		ts, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:   true,
			BatchStore: mock.New(),
		})
		jsonhttptest.Request(t, ts, http.MethodGet, "/reservestate", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&api.ReserveStateResponse{}),
		)
	})
}
func TestChainState(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		cs := &postage.ChainState{
			Block:        123456,
			TotalAmount:  big.NewInt(50),
			CurrentPrice: big.NewInt(5),
		}
		ts, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:   true,
			BatchStore: mock.New(mock.WithChainState(cs)),
		})
		jsonhttptest.Request(t, ts, http.MethodGet, "/chainstate", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&api.ChainStateResponse{
				Block:        123456,
				TotalAmount:  bigint.Wrap(big.NewInt(50)),
				CurrentPrice: bigint.Wrap(big.NewInt(5)),
			}),
		)
	})

	t.Run("empty", func(t *testing.T) {
		ts, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:   true,
			BatchStore: mock.New(),
		})
		jsonhttptest.Request(t, ts, http.MethodGet, "/chainstate", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&api.ChainStateResponse{}),
		)
	})
}

func TestPostageTopUpStamp(t *testing.T) {
	topupAmount := int64(1000)
	topupBatch := func(id string, amount int64) string {
		return fmt.Sprintf("/stamps/topup/%s/%d", id, amount)
	}

	t.Run("ok", func(t *testing.T) {
		contract := contractMock.New(
			contractMock.WithTopUpBatchFunc(func(ctx context.Context, id []byte, ib *big.Int) error {
				if !bytes.Equal(id, batchOk) {
					return errors.New("incorrect batch ID in call")
				}
				if ib.Cmp(big.NewInt(topupAmount)) != 0 {
					return fmt.Errorf("called with wrong topup amount. wanted %d, got %d", topupAmount, ib)
				}
				return nil
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:        true,
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPatch, topupBatch(batchOkStr, topupAmount), http.StatusAccepted,
			jsonhttptest.WithExpectedJSONResponse(&api.PostageCreateResponse{
				BatchID: batchOk,
			}),
		)
	})

	t.Run("with-custom-gas", func(t *testing.T) {
		contract := contractMock.New(
			contractMock.WithTopUpBatchFunc(func(ctx context.Context, id []byte, ib *big.Int) error {
				if !bytes.Equal(id, batchOk) {
					return errors.New("incorrect batch ID in call")
				}
				if ib.Cmp(big.NewInt(topupAmount)) != 0 {
					return fmt.Errorf("called with wrong topup amount. wanted %d, got %d", topupAmount, ib)
				}
				if sctx.GetGasPrice(ctx).Cmp(big.NewInt(10000)) != 0 {
					return fmt.Errorf("called with wrong gas price. wanted %d, got %d", 10000, sctx.GetGasPrice(ctx))
				}
				return nil
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:        true,
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPatch, topupBatch(batchOkStr, topupAmount), http.StatusAccepted,
			jsonhttptest.WithRequestHeader("Gas-Price", "10000"),
			jsonhttptest.WithExpectedJSONResponse(&api.PostageCreateResponse{
				BatchID: batchOk,
			}),
		)
	})

	t.Run("with-error", func(t *testing.T) {
		contract := contractMock.New(
			contractMock.WithTopUpBatchFunc(func(ctx context.Context, id []byte, ib *big.Int) error {
				return errors.New("err")
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:        true,
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPatch, topupBatch(batchOkStr, topupAmount), http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusInternalServerError,
				Message: "cannot topup batch",
			}),
		)
	})

	t.Run("out-of-funds", func(t *testing.T) {
		contract := contractMock.New(
			contractMock.WithTopUpBatchFunc(func(ctx context.Context, id []byte, ib *big.Int) error {
				return postagecontract.ErrInsufficientFunds
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:        true,
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPatch, topupBatch(batchOkStr, topupAmount), http.StatusPaymentRequired,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusPaymentRequired,
				Message: "out of funds",
			}),
		)
	})

	t.Run("invalid batch id", func(t *testing.T) {
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true})

		jsonhttptest.Request(t, ts, http.MethodPatch, "/stamps/topup/abcd/2", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid batchID",
			}),
		)
	})

	t.Run("invalid amount", func(t *testing.T) {
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true})

		wrongURL := fmt.Sprintf("/stamps/topup/%s/amount", batchOkStr)

		jsonhttptest.Request(t, ts, http.MethodPatch, wrongURL, http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid postage amount",
			}),
		)
	})
}

func TestPostageDiluteStamp(t *testing.T) {
	newBatchDepth := uint8(17)
	diluteBatch := func(id string, depth uint8) string {
		return fmt.Sprintf("/stamps/dilute/%s/%d", id, depth)
	}

	t.Run("ok", func(t *testing.T) {
		contract := contractMock.New(
			contractMock.WithDiluteBatchFunc(func(ctx context.Context, id []byte, newDepth uint8) error {
				if !bytes.Equal(id, batchOk) {
					return errors.New("incorrect batch ID in call")
				}
				if newDepth != newBatchDepth {
					return fmt.Errorf("called with wrong depth. wanted %d, got %d", newBatchDepth, newDepth)
				}
				return nil
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:        true,
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPatch, diluteBatch(batchOkStr, newBatchDepth), http.StatusAccepted,
			jsonhttptest.WithExpectedJSONResponse(&api.PostageCreateResponse{
				BatchID: batchOk,
			}),
		)
	})

	t.Run("with-custom-gas", func(t *testing.T) {
		contract := contractMock.New(
			contractMock.WithDiluteBatchFunc(func(ctx context.Context, id []byte, newDepth uint8) error {
				if !bytes.Equal(id, batchOk) {
					return errors.New("incorrect batch ID in call")
				}
				if newDepth != newBatchDepth {
					return fmt.Errorf("called with wrong depth. wanted %d, got %d", newBatchDepth, newDepth)
				}
				if sctx.GetGasPrice(ctx).Cmp(big.NewInt(10000)) != 0 {
					return fmt.Errorf("called with wrong gas price. wanted %d, got %d", 10000, sctx.GetGasPrice(ctx))
				}
				return nil
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:        true,
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPatch, diluteBatch(batchOkStr, newBatchDepth), http.StatusAccepted,
			jsonhttptest.WithRequestHeader("Gas-Price", "10000"),
			jsonhttptest.WithExpectedJSONResponse(&api.PostageCreateResponse{
				BatchID: batchOk,
			}),
		)
	})

	t.Run("with-error", func(t *testing.T) {
		contract := contractMock.New(
			contractMock.WithDiluteBatchFunc(func(ctx context.Context, id []byte, newDepth uint8) error {
				return errors.New("err")
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:        true,
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPatch, diluteBatch(batchOkStr, newBatchDepth), http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusInternalServerError,
				Message: "cannot dilute batch",
			}),
		)
	})

	t.Run("with depth error", func(t *testing.T) {
		contract := contractMock.New(
			contractMock.WithDiluteBatchFunc(func(ctx context.Context, id []byte, newDepth uint8) error {
				return postagecontract.ErrInvalidDepth
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:        true,
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPatch, diluteBatch(batchOkStr, newBatchDepth), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid depth",
			}),
		)
	})

	t.Run("invalid batch id", func(t *testing.T) {
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true})

		jsonhttptest.Request(t, ts, http.MethodPatch, "/stamps/dilute/abcd/2", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid batchID",
			}),
		)
	})

	t.Run("invalid depth", func(t *testing.T) {
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true})

		wrongURL := fmt.Sprintf("/stamps/dilute/%s/depth", batchOkStr)

		jsonhttptest.Request(t, ts, http.MethodPatch, wrongURL, http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid depth",
			}),
		)
	})
}

// Tests the postageAccessHandler middleware for any set of operations that are guarded
// by the postage semaphore
func TestPostageAccessHandler(t *testing.T) {

	type operation struct {
		name     string
		method   string
		url      string
		respCode int
		resp     interface{}
	}

	success := []operation{
		{
			name:     "create batch ok",
			method:   http.MethodPost,
			url:      "/stamps/1000/17?label=test",
			respCode: http.StatusCreated,
			resp: &api.PostageCreateResponse{
				BatchID: batchOk,
			},
		},
		{
			name:     "topup batch ok",
			method:   http.MethodPatch,
			url:      fmt.Sprintf("/stamps/topup/%s/10", batchOkStr),
			respCode: http.StatusAccepted,
			resp: &api.PostageCreateResponse{
				BatchID: batchOk,
			},
		},
		{
			name:     "dilute batch ok",
			method:   http.MethodPatch,
			url:      fmt.Sprintf("/stamps/dilute/%s/18", batchOkStr),
			respCode: http.StatusAccepted,
			resp: &api.PostageCreateResponse{
				BatchID: batchOk,
			},
		},
	}

	failure := []operation{
		{
			name:     "create batch not ok",
			method:   http.MethodPost,
			url:      "/stamps/1000/17?label=test",
			respCode: http.StatusTooManyRequests,
			resp: &jsonhttp.StatusResponse{
				Code:    http.StatusTooManyRequests,
				Message: "simultaneous on-chain operations not supported",
			},
		},
		{
			name:     "topup batch not ok",
			method:   http.MethodPatch,
			url:      fmt.Sprintf("/stamps/topup/%s/10", batchOkStr),
			respCode: http.StatusTooManyRequests,
			resp: &jsonhttp.StatusResponse{
				Code:    http.StatusTooManyRequests,
				Message: "simultaneous on-chain operations not supported",
			},
		},
		{
			name:     "dilute batch not ok",
			method:   http.MethodPatch,
			url:      fmt.Sprintf("/stamps/dilute/%s/18", batchOkStr),
			respCode: http.StatusTooManyRequests,
			resp: &jsonhttp.StatusResponse{
				Code:    http.StatusTooManyRequests,
				Message: "simultaneous on-chain operations not supported",
			},
		},
	}

	for _, op1 := range success {
		for _, op2 := range failure {

			t.Run(op1.name+"-"+op2.name, func(t *testing.T) {
				wait, done := make(chan struct{}), make(chan struct{})
				contract := contractMock.New(
					contractMock.WithCreateBatchFunc(func(ctx context.Context, ib *big.Int, d uint8, i bool, l string) ([]byte, error) {
						<-wait
						return batchOk, nil
					}),
					contractMock.WithTopUpBatchFunc(func(ctx context.Context, id []byte, ib *big.Int) error {
						<-wait
						return nil
					}),
					contractMock.WithDiluteBatchFunc(func(ctx context.Context, id []byte, newDepth uint8) error {
						<-wait
						return nil
					}),
				)

				ts, _, _, _ := newTestServer(t, testServerOptions{
					DebugAPI:        true,
					PostageContract: contract,
				})

				go func() {
					defer close(done)

					jsonhttptest.Request(t, ts, op1.method, op1.url, op1.respCode, jsonhttptest.WithExpectedJSONResponse(op1.resp))
				}()

				time.Sleep(time.Millisecond * 100)

				jsonhttptest.Request(t, ts, op2.method, op2.url, op2.respCode, jsonhttptest.WithExpectedJSONResponse(op2.resp))

				close(wait)
				<-done
			})
		}
	}
}
