// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/debugapi"
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
		ts := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts.Client, http.MethodPost, createBatch(initialBalance, depth, label), http.StatusCreated,
			jsonhttptest.WithExpectedJSONResponse(&debugapi.PostageCreateResponse{
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
		ts := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts.Client, http.MethodPost, createBatch(initialBalance, depth, label), http.StatusCreated,
			jsonhttptest.WithRequestHeader("Gas-Price", "10000"),
			jsonhttptest.WithExpectedJSONResponse(&debugapi.PostageCreateResponse{
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
		ts := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts.Client, http.MethodPost, createBatch(initialBalance, depth, label), http.StatusInternalServerError,
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
		ts := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts.Client, http.MethodPost, createBatch(initialBalance, depth, label), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "out of funds",
			}),
		)
	})

	t.Run("invalid depth", func(t *testing.T) {
		ts := newTestServer(t, testServerOptions{})

		jsonhttptest.Request(t, ts.Client, http.MethodPost, "/stamps/1000/ab", http.StatusBadRequest,
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
		ts := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts.Client, http.MethodPost, "/stamps/1000/9", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid depth",
			}),
		)
	})

	t.Run("invalid balance", func(t *testing.T) {
		ts := newTestServer(t, testServerOptions{})

		jsonhttptest.Request(t, ts.Client, http.MethodPost, "/stamps/abcd/2", http.StatusBadRequest,
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
		ts := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts.Client, http.MethodPost, "/stamps/1000/24", http.StatusCreated,
			jsonhttptest.WithRequestHeader("Immutable", "true"),
			jsonhttptest.WithExpectedJSONResponse(&debugapi.PostageCreateResponse{
				BatchID: batchID,
			}),
		)

		if !immutable {
			t.Fatalf("want true, got %v", immutable)
		}

	})
}

func TestPostageGetStamps(t *testing.T) {
	b := postagetesting.MustNewBatch()
	b.Value = big.NewInt(20)
	si := postage.NewStampIssuer("", "", b.ID, big.NewInt(3), 11, 10, 1000, true)
	mp := mockpost.New(mockpost.WithIssuer(si))
	cs := &postage.ChainState{Block: 10, TotalAmount: big.NewInt(5), CurrentPrice: big.NewInt(2)}
	bs := mock.New(mock.WithChainState(cs), mock.WithBatch(b))
	ts := newTestServer(t, testServerOptions{Post: mp, BatchStore: bs})

	jsonhttptest.Request(t, ts.Client, http.MethodGet, "/stamps", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(&debugapi.PostageStampsResponse{
			Stamps: []debugapi.PostageStampResponse{
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
}

func TestPostageGetStamp(t *testing.T) {
	b := postagetesting.MustNewBatch()
	b.Value = big.NewInt(20)
	si := postage.NewStampIssuer("", "", b.ID, big.NewInt(3), 11, 10, 1000, true)
	mp := mockpost.New(mockpost.WithIssuer(si))
	cs := &postage.ChainState{Block: 10, TotalAmount: big.NewInt(5), CurrentPrice: big.NewInt(2)}
	bs := mock.New(mock.WithChainState(cs), mock.WithBatch(b))
	ts := newTestServer(t, testServerOptions{Post: mp, BatchStore: bs})

	t.Run("ok", func(t *testing.T) {
		jsonhttptest.Request(t, ts.Client, http.MethodGet, "/stamps/"+hex.EncodeToString(b.ID), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&debugapi.PostageStampResponse{
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

		jsonhttptest.Request(t, ts.Client, http.MethodGet, "/stamps/"+hex.EncodeToString(badBatch), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid batchID",
			}),
		)
	})
	t.Run("bad request", func(t *testing.T) {
		badBatch := []byte{0, 1, 2, 4}

		jsonhttptest.Request(t, ts.Client, http.MethodGet, "/stamps/"+hex.EncodeToString(badBatch), http.StatusBadRequest,
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
	ts := newTestServer(t, testServerOptions{Post: mp})
	buckets := make([]debugapi.BucketData, 1024)
	for i := range buckets {
		buckets[i] = debugapi.BucketData{BucketID: uint32(i)}
	}

	t.Run("ok", func(t *testing.T) {
		jsonhttptest.Request(t, ts.Client, http.MethodGet, "/stamps/"+batchOkStr+"/buckets", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&debugapi.PostageStampBucketsResponse{
				Depth:            si.Depth(),
				BucketDepth:      si.BucketDepth(),
				BucketUpperBound: si.BucketUpperBound(),
				Buckets:          buckets,
			}),
		)
	})
	t.Run("bad batch", func(t *testing.T) {
		badBatch := []byte{0, 1, 2}

		jsonhttptest.Request(t, ts.Client, http.MethodGet, "/stamps/"+hex.EncodeToString(badBatch)+"/buckets", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid batchID",
			}),
		)
	})
	t.Run("bad batch", func(t *testing.T) {
		badBatch := []byte{0, 1, 2, 4}

		jsonhttptest.Request(t, ts.Client, http.MethodGet, "/stamps/"+hex.EncodeToString(badBatch)+"/buckets", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid batchID",
			}),
		)
	})
}

func TestReserveState(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		ts := newTestServer(t, testServerOptions{
			BatchStore: mock.New(mock.WithReserveState(&postage.ReserveState{
				Radius: 5,
				Outer:  big.NewInt(5),
				Inner:  big.NewInt(5),
			})),
		})
		jsonhttptest.Request(t, ts.Client, http.MethodGet, "/reservestate", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&debugapi.ReserveStateResponse{
				Radius: 5,
				Outer:  bigint.Wrap(big.NewInt(5)),
				Inner:  bigint.Wrap(big.NewInt(5)),
			}),
		)
	})
	t.Run("empty", func(t *testing.T) {
		ts := newTestServer(t, testServerOptions{
			BatchStore: mock.New(),
		})
		jsonhttptest.Request(t, ts.Client, http.MethodGet, "/reservestate", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&debugapi.ReserveStateResponse{}),
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
		ts := newTestServer(t, testServerOptions{
			BatchStore: mock.New(mock.WithChainState(cs)),
		})
		jsonhttptest.Request(t, ts.Client, http.MethodGet, "/chainstate", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&debugapi.ChainStateResponse{
				Block:        123456,
				TotalAmount:  bigint.Wrap(big.NewInt(50)),
				CurrentPrice: bigint.Wrap(big.NewInt(5)),
			}),
		)
	})

	t.Run("empty", func(t *testing.T) {
		ts := newTestServer(t, testServerOptions{
			BatchStore: mock.New(),
		})
		jsonhttptest.Request(t, ts.Client, http.MethodGet, "/chainstate", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&debugapi.ChainStateResponse{}),
		)
	})
}
