// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package debugapi_test

import (
	"bytes"
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
		ts := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts.Client, http.MethodPatch, topupBatch(batchOkStr, topupAmount), http.StatusAccepted,
			jsonhttptest.WithExpectedJSONResponse(&debugapi.PostageCreateResponse{
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
		ts := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts.Client, http.MethodPatch, topupBatch(batchOkStr, topupAmount), http.StatusAccepted,
			jsonhttptest.WithRequestHeader("Gas-Price", "10000"),
			jsonhttptest.WithExpectedJSONResponse(&debugapi.PostageCreateResponse{
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
		ts := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts.Client, http.MethodPatch, topupBatch(batchOkStr, topupAmount), http.StatusInternalServerError,
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
		ts := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts.Client, http.MethodPatch, topupBatch(batchOkStr, topupAmount), http.StatusPaymentRequired,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusPaymentRequired,
				Message: "out of funds",
			}),
		)
	})

	t.Run("invalid batch id", func(t *testing.T) {
		ts := newTestServer(t, testServerOptions{})

		jsonhttptest.Request(t, ts.Client, http.MethodPatch, "/stamps/topup/abcd/2", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid batchID",
			}),
		)
	})

	t.Run("invalid amount", func(t *testing.T) {
		ts := newTestServer(t, testServerOptions{})

		wrongURL := fmt.Sprintf("/stamps/topup/%s/amount", batchOkStr)

		jsonhttptest.Request(t, ts.Client, http.MethodPatch, wrongURL, http.StatusBadRequest,
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
		ts := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts.Client, http.MethodPatch, diluteBatch(batchOkStr, newBatchDepth), http.StatusAccepted,
			jsonhttptest.WithExpectedJSONResponse(&debugapi.PostageCreateResponse{
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
		ts := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts.Client, http.MethodPatch, diluteBatch(batchOkStr, newBatchDepth), http.StatusAccepted,
			jsonhttptest.WithRequestHeader("Gas-Price", "10000"),
			jsonhttptest.WithExpectedJSONResponse(&debugapi.PostageCreateResponse{
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
		ts := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts.Client, http.MethodPatch, diluteBatch(batchOkStr, newBatchDepth), http.StatusInternalServerError,
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
		ts := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts.Client, http.MethodPatch, diluteBatch(batchOkStr, newBatchDepth), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid depth",
			}),
		)
	})

	t.Run("invalid batch id", func(t *testing.T) {
		ts := newTestServer(t, testServerOptions{})

		jsonhttptest.Request(t, ts.Client, http.MethodPatch, "/stamps/dilute/abcd/2", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid batchID",
			}),
		)
	})

	t.Run("invalid depth", func(t *testing.T) {
		ts := newTestServer(t, testServerOptions{})

		wrongURL := fmt.Sprintf("/stamps/dilute/%s/depth", batchOkStr)

		jsonhttptest.Request(t, ts.Client, http.MethodPatch, wrongURL, http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid depth",
			}),
		)
	})
}
