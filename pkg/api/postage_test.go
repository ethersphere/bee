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
	"strconv"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/bigint"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	mockpost "github.com/ethersphere/bee/v2/pkg/postage/mock"
	"github.com/ethersphere/bee/v2/pkg/postage/postagecontract"
	contractMock "github.com/ethersphere/bee/v2/pkg/postage/postagecontract/mock"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	"github.com/ethersphere/bee/v2/pkg/sctx"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/transaction/backendmock"
)

func TestPostageCreateStamp(t *testing.T) {
	t.Parallel()

	batchID := []byte{1, 2, 3, 4}
	initialBalance := int64(1000)
	depth := uint8(24)
	label := "label"
	txHash := common.HexToHash("0x1234")
	createBatch := func(amount int64, depth uint8, label string) string {
		return fmt.Sprintf("/stamps/%d/%d?label=%s", amount, depth, label)
	}

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		var immutable bool
		contract := contractMock.New(
			contractMock.WithCreateBatchFunc(func(ctx context.Context, ib *big.Int, d uint8, i bool, l string) (common.Hash, []byte, error) {
				if ib.Cmp(big.NewInt(initialBalance)) != 0 {
					return common.Hash{}, nil, fmt.Errorf("called with wrong initial balance. wanted %d, got %d", initialBalance, ib)
				}
				immutable = i
				if d != depth {
					return common.Hash{}, nil, fmt.Errorf("called with wrong depth. wanted %d, got %d", depth, d)
				}
				if l != label {
					return common.Hash{}, nil, fmt.Errorf("called with wrong label. wanted %s, got %s", label, l)
				}
				return txHash, batchID, nil
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPost, createBatch(initialBalance, depth, label), http.StatusCreated,
			jsonhttptest.WithExpectedJSONResponse(&api.PostageCreateResponse{
				BatchID: batchID,
				TxHash:  txHash.String(),
			}),
		)

		if !immutable {
			t.Fatalf("default batch should be immutable")
		}
	})

	t.Run("with-error", func(t *testing.T) {
		t.Parallel()

		contract := contractMock.New(
			contractMock.WithCreateBatchFunc(func(ctx context.Context, ib *big.Int, d uint8, i bool, l string) (common.Hash, []byte, error) {
				return common.Hash{}, nil, errors.New("err")
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
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
		t.Parallel()

		contract := contractMock.New(
			contractMock.WithCreateBatchFunc(func(ctx context.Context, ib *big.Int, d uint8, i bool, l string) (common.Hash, []byte, error) {
				return common.Hash{}, nil, postagecontract.ErrInsufficientFunds
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPost, createBatch(initialBalance, depth, label), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "out of funds",
			}),
		)
	})

	t.Run("depth less than bucket depth", func(t *testing.T) {
		t.Parallel()

		contract := contractMock.New(
			contractMock.WithCreateBatchFunc(func(ctx context.Context, ib *big.Int, d uint8, i bool, l string) (common.Hash, []byte, error) {
				return common.Hash{}, nil, postagecontract.ErrInvalidDepth
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPost, "/stamps/1000/9", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid path params",
				Reasons: []jsonhttp.Reason{
					{
						Field: "depth",
						Error: "want min:17",
					},
				},
			}),
		)
	})

	t.Run("mutable header", func(t *testing.T) {
		t.Parallel()

		var immutable bool
		contract := contractMock.New(
			contractMock.WithCreateBatchFunc(func(ctx context.Context, _ *big.Int, _ uint8, i bool, _ string) (common.Hash, []byte, error) {
				immutable = i
				return txHash, batchID, nil
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPost, "/stamps/1000/24", http.StatusCreated,
			jsonhttptest.WithRequestHeader(api.ImmutableHeader, "false"),
			jsonhttptest.WithExpectedJSONResponse(&api.PostageCreateResponse{
				BatchID: batchID,
				TxHash:  txHash.String(),
			}),
		)

		if immutable {
			t.Fatalf("want false, got %v", immutable)
		}
	})

	t.Run("syncing in progress", func(t *testing.T) {
		t.Parallel()

		ts, _, _, _ := newTestServer(t, testServerOptions{
			SyncStatus: func() (bool, error) { return false, nil },
		})

		jsonhttptest.Request(t, ts, http.MethodPost, createBatch(initialBalance, depth, label), http.StatusServiceUnavailable,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Message: "syncing in progress",
				Code:    503,
			}),
		)
	})
	t.Run("syncing failed", func(t *testing.T) {
		t.Parallel()

		ts, _, _, _ := newTestServer(t, testServerOptions{
			SyncStatus: func() (bool, error) { return true, errors.New("oops") },
		})

		jsonhttptest.Request(t, ts, http.MethodPost, createBatch(initialBalance, depth, label), http.StatusServiceUnavailable,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Message: "syncing failed",
				Code:    503,
			}),
		)
	})
}

func TestPostageGetStamps(t *testing.T) {
	t.Parallel()

	b := postagetesting.MustNewBatch(postagetesting.WithValue(20))

	si := postage.NewStampIssuer("", "", b.ID, big.NewInt(3), 11, 10, 1000, true)
	mp := mockpost.New(mockpost.WithIssuer(si))
	cs := &postage.ChainState{Block: 10, TotalAmount: big.NewInt(5), CurrentPrice: big.NewInt(2)}

	t.Run("single stamp", func(t *testing.T) {
		t.Parallel()

		bs := mock.New(mock.WithChainState(cs), mock.WithBatch(b))
		ts, _, _, _ := newTestServer(t, testServerOptions{Post: mp, BatchStore: bs, BlockTime: 2 * time.Second})

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

	t.Run("single expired Stamp", func(t *testing.T) {
		t.Parallel()

		eb := postagetesting.MustNewBatch(postagetesting.WithValue(20))

		esi := postage.NewStampIssuer("", "", eb.ID, big.NewInt(3), 11, 10, 1000, true)
		emp := mockpost.New(mockpost.WithIssuer(esi))
		err := emp.HandleStampExpiry(context.Background(), eb.ID)
		if err != nil {
			t.Fatal(err)
		}
		ecs := &postage.ChainState{Block: 10, TotalAmount: big.NewInt(15), CurrentPrice: big.NewInt(12)}
		ebs := mock.New(mock.WithChainState(ecs))
		ts, _, _, _ := newTestServer(t, testServerOptions{Post: emp, BatchStore: ebs, BlockTime: 2 * time.Second})

		jsonhttptest.Request(t, ts, http.MethodGet, "/stamps/"+hex.EncodeToString(eb.ID), http.StatusNotFound,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Message: "issuer does not exist",
				Code:    404,
			}),
		)
	})
}

// TestGetAllBatches tests that the endpoint that returns all living
// batches functions correctly.
func TestGetAllBatches(t *testing.T) {
	t.Parallel()

	b := postagetesting.MustNewBatch()
	b.Value = big.NewInt(20)
	si := postage.NewStampIssuer("", "", b.ID, big.NewInt(3), 11, 10, 1000, true)
	mp := mockpost.New(mockpost.WithIssuer(si))
	cs := &postage.ChainState{Block: 10, TotalAmount: big.NewInt(5), CurrentPrice: big.NewInt(2)}
	bs := mock.New(mock.WithChainState(cs), mock.WithBatch(b))
	ts, _, _, _ := newTestServer(t, testServerOptions{Post: mp, BatchStore: bs, BlockTime: 2 * time.Second})

	oneBatch := struct {
		Batches []api.PostageBatchResponse `json:"batches"`
	}{
		Batches: []api.PostageBatchResponse{
			{
				BatchID:     b.ID,
				Value:       bigint.Wrap(b.Value),
				Start:       b.Start,
				Owner:       b.Owner,
				Depth:       b.Depth,
				BucketDepth: b.BucketDepth,
				Immutable:   b.Immutable,
				BatchTTL:    15, // ((value-totalAmount)/pricePerBlock)*blockTime=((20-5)/2)*2.
			},
		},
	}

	t.Run("all stamps", func(t *testing.T) {
		t.Parallel()

		jsonhttptest.Request(t, ts, http.MethodGet, "/batches", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(oneBatch),
		)
	})
}

func TestPostageGetStamp(t *testing.T) {
	t.Parallel()

	b := postagetesting.MustNewBatch()
	b.Value = big.NewInt(20)
	si := postage.NewStampIssuer("", "", b.ID, big.NewInt(3), 11, 10, 1000, true)
	mp := mockpost.New(mockpost.WithIssuer(si))
	cs := &postage.ChainState{Block: 10, TotalAmount: big.NewInt(5), CurrentPrice: big.NewInt(2)}
	bs := mock.New(mock.WithChainState(cs), mock.WithBatch(b))
	ts, _, _, _ := newTestServer(t, testServerOptions{Post: mp, BatchStore: bs, BlockTime: 2 * time.Second})

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

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
}

func TestPostageGetBuckets(t *testing.T) {
	t.Parallel()

	si := postage.NewStampIssuer("", "", batchOk, big.NewInt(3), 11, 10, 1000, true)
	mp := mockpost.New(mockpost.WithIssuer(si))
	ts, _, _, _ := newTestServer(t, testServerOptions{Post: mp})
	buckets := make([]api.BucketData, 1024)
	for i := range buckets {
		buckets[i] = api.BucketData{BucketID: uint32(i)}
	}

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		jsonhttptest.Request(t, ts, http.MethodGet, "/stamps/"+batchOkStr+"/buckets", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&api.PostageStampBucketsResponse{
				Depth:            si.Depth(),
				BucketDepth:      si.BucketDepth(),
				BucketUpperBound: si.BucketUpperBound(),
				Buckets:          buckets,
			}),
		)
	})

	t.Run("batch not found", func(t *testing.T) {
		t.Parallel()

		mpNotFound := mockpost.New()
		tsNotFound, _, _, _ := newTestServer(t, testServerOptions{Post: mpNotFound})

		jsonhttptest.Request(t, tsNotFound, http.MethodGet, "/stamps/"+batchOkStr+"/buckets", http.StatusNotFound)
	})

}

func TestReserveState(t *testing.T) {
	t.Parallel()

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		ts, _, _, _ := newTestServer(t, testServerOptions{
			BatchStore: mock.New(mock.WithRadius(5)),
			Storer:     mockstorer.New(),
		})
		jsonhttptest.Request(t, ts, http.MethodGet, "/reservestate", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&api.ReserveStateResponse{
				Radius: 5,
			}),
		)
	})
	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		ts, _, _, _ := newTestServer(t, testServerOptions{
			BatchStore: mock.New(),
			Storer:     mockstorer.New(),
		})
		jsonhttptest.Request(t, ts, http.MethodGet, "/reservestate", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&api.ReserveStateResponse{}),
		)
	})
}
func TestChainState(t *testing.T) {
	t.Parallel()

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		cs := &postage.ChainState{
			Block:        123456,
			TotalAmount:  big.NewInt(50),
			CurrentPrice: big.NewInt(5),
		}
		ts, _, _, _ := newTestServer(t, testServerOptions{
			BatchStore: mock.New(mock.WithChainState(cs)),
			BackendOpts: []backendmock.Option{backendmock.WithBlockNumberFunc(func(ctx context.Context) (uint64, error) {
				return 1, nil
			})},
		})
		jsonhttptest.Request(t, ts, http.MethodGet, "/chainstate", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&api.ChainStateResponse{
				ChainTip:     1,
				Block:        123456,
				TotalAmount:  bigint.Wrap(big.NewInt(50)),
				CurrentPrice: bigint.Wrap(big.NewInt(5)),
			}),
		)
	})

}

func TestPostageTopUpStamp(t *testing.T) {
	t.Parallel()

	txHash := common.HexToHash("0x1234")
	topupAmount := int64(1000)
	topupBatch := func(id string, amount int64) string {
		return fmt.Sprintf("/stamps/topup/%s/%d", id, amount)
	}

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		contract := contractMock.New(
			contractMock.WithTopUpBatchFunc(func(ctx context.Context, id []byte, ib *big.Int) (common.Hash, error) {
				if !bytes.Equal(id, batchOk) {
					return common.Hash{}, errors.New("incorrect batch ID in call")
				}
				if ib.Cmp(big.NewInt(topupAmount)) != 0 {
					return common.Hash{}, fmt.Errorf("called with wrong topup amount. wanted %d, got %d", topupAmount, ib)
				}
				return txHash, nil
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPatch, topupBatch(batchOkStr, topupAmount), http.StatusAccepted,
			jsonhttptest.WithExpectedJSONResponse(&api.PostageCreateResponse{
				BatchID: batchOk,
				TxHash:  txHash.String(),
			}),
		)
	})

	t.Run("with-custom-gas", func(t *testing.T) {
		t.Parallel()

		contract := contractMock.New(
			contractMock.WithTopUpBatchFunc(func(ctx context.Context, id []byte, ib *big.Int) (common.Hash, error) {
				if !bytes.Equal(id, batchOk) {
					return common.Hash{}, errors.New("incorrect batch ID in call")
				}
				if ib.Cmp(big.NewInt(topupAmount)) != 0 {
					return common.Hash{}, fmt.Errorf("called with wrong topup amount. wanted %d, got %d", topupAmount, ib)
				}
				if sctx.GetGasPrice(ctx).Cmp(big.NewInt(10000)) != 0 {
					return common.Hash{}, fmt.Errorf("called with wrong gas price. wanted %d, got %d", 10000, sctx.GetGasPrice(ctx))
				}
				return txHash, nil
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPatch, topupBatch(batchOkStr, topupAmount), http.StatusAccepted,
			jsonhttptest.WithRequestHeader(api.GasPriceHeader, "10000"),
			jsonhttptest.WithExpectedJSONResponse(&api.PostageCreateResponse{
				BatchID: batchOk,
				TxHash:  txHash.String(),
			}),
		)
	})

	t.Run("with-error", func(t *testing.T) {
		t.Parallel()

		contract := contractMock.New(
			contractMock.WithTopUpBatchFunc(func(ctx context.Context, id []byte, ib *big.Int) (common.Hash, error) {
				return common.Hash{}, errors.New("err")
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
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
		t.Parallel()

		contract := contractMock.New(
			contractMock.WithTopUpBatchFunc(func(ctx context.Context, id []byte, ib *big.Int) (common.Hash, error) {
				return common.Hash{}, postagecontract.ErrInsufficientFunds
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPatch, topupBatch(batchOkStr, topupAmount), http.StatusPaymentRequired,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusPaymentRequired,
				Message: "out of funds",
			}),
		)
	})

	t.Run("gas limit header", func(t *testing.T) {
		t.Parallel()

		contract := contractMock.New(
			contractMock.WithTopUpBatchFunc(func(ctx context.Context, id []byte, ib *big.Int) (common.Hash, error) {
				if sctx.GetGasLimit(ctx) != 10000 {
					return common.Hash{}, fmt.Errorf("called with wrong gas price. wanted %d, got %d", 10000, sctx.GetGasLimit(ctx))
				}
				return txHash, nil
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPatch, topupBatch(batchOkStr, topupAmount), http.StatusAccepted,
			jsonhttptest.WithRequestHeader(api.GasLimitHeader, "10000"),
			jsonhttptest.WithExpectedJSONResponse(&api.PostageCreateResponse{
				BatchID: batchOk,
				TxHash:  txHash.String(),
			}),
		)
	})
}

func TestPostageDiluteStamp(t *testing.T) {
	t.Parallel()

	txHash := common.HexToHash("0x1234")
	newBatchDepth := uint8(17)
	diluteBatch := func(id string, depth uint8) string {
		return fmt.Sprintf("/stamps/dilute/%s/%d", id, depth)
	}

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		contract := contractMock.New(
			contractMock.WithDiluteBatchFunc(func(ctx context.Context, id []byte, newDepth uint8) (common.Hash, error) {
				if !bytes.Equal(id, batchOk) {
					return common.Hash{}, errors.New("incorrect batch ID in call")
				}
				if newDepth != newBatchDepth {
					return common.Hash{}, fmt.Errorf("called with wrong depth. wanted %d, got %d", newBatchDepth, newDepth)
				}
				return txHash, nil
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPatch, diluteBatch(batchOkStr, newBatchDepth), http.StatusAccepted,
			jsonhttptest.WithExpectedJSONResponse(&api.PostageCreateResponse{
				BatchID: batchOk,
				TxHash:  txHash.String(),
			}),
		)
	})

	t.Run("with-custom-gas", func(t *testing.T) {
		t.Parallel()

		contract := contractMock.New(
			contractMock.WithDiluteBatchFunc(func(ctx context.Context, id []byte, newDepth uint8) (common.Hash, error) {
				if !bytes.Equal(id, batchOk) {
					return common.Hash{}, errors.New("incorrect batch ID in call")
				}
				if newDepth != newBatchDepth {
					return common.Hash{}, fmt.Errorf("called with wrong depth. wanted %d, got %d", newBatchDepth, newDepth)
				}
				if sctx.GetGasPrice(ctx).Cmp(big.NewInt(10000)) != 0 {
					return common.Hash{}, fmt.Errorf("called with wrong gas price. wanted %d, got %d", 10000, sctx.GetGasPrice(ctx))
				}
				return txHash, nil
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPatch, diluteBatch(batchOkStr, newBatchDepth), http.StatusAccepted,
			jsonhttptest.WithRequestHeader(api.GasPriceHeader, "10000"),
			jsonhttptest.WithExpectedJSONResponse(&api.PostageCreateResponse{
				BatchID: batchOk,
				TxHash:  txHash.String(),
			}),
		)
	})

	t.Run("with-error", func(t *testing.T) {
		t.Parallel()

		contract := contractMock.New(
			contractMock.WithDiluteBatchFunc(func(ctx context.Context, id []byte, newDepth uint8) (common.Hash, error) {
				return common.Hash{}, errors.New("err")
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
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
		t.Parallel()

		contract := contractMock.New(
			contractMock.WithDiluteBatchFunc(func(ctx context.Context, id []byte, newDepth uint8) (common.Hash, error) {
				return common.Hash{}, postagecontract.ErrInvalidDepth
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPatch, diluteBatch(batchOkStr, newBatchDepth), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid depth",
			}),
		)
	})

	t.Run("gas limit header", func(t *testing.T) {
		t.Parallel()

		contract := contractMock.New(
			contractMock.WithDiluteBatchFunc(func(ctx context.Context, _ []byte, _ uint8) (common.Hash, error) {
				if sctx.GetGasLimit(ctx) != 10000 {
					return common.Hash{}, fmt.Errorf("called with wrong gas price. wanted %d, got %d", 10000, sctx.GetGasLimit(ctx))
				}
				return txHash, nil
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPatch, diluteBatch(batchOkStr, newBatchDepth), http.StatusAccepted,
			jsonhttptest.WithRequestHeader(api.GasLimitHeader, "10000"),
			jsonhttptest.WithExpectedJSONResponse(&api.PostageCreateResponse{
				BatchID: batchOk,
				TxHash:  txHash.String(),
			}),
		)

	})
}

// Tests the postageAccessHandler middleware for any set of operations that are guarded
// by the postage semaphore
func TestPostageAccessHandler(t *testing.T) {
	t.Parallel()

	txHash := common.HexToHash("0x1234")

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
			url:      "/stamps/1000/24?label=test",
			respCode: http.StatusCreated,
			resp: &api.PostageCreateResponse{
				BatchID: batchOk,
				TxHash:  txHash.String(),
			},
		},
		{
			name:     "topup batch ok",
			method:   http.MethodPatch,
			url:      fmt.Sprintf("/stamps/topup/%s/10", batchOkStr),
			respCode: http.StatusAccepted,
			resp: &api.PostageCreateResponse{
				BatchID: batchOk,
				TxHash:  txHash.String(),
			},
		},
		{
			name:     "dilute batch ok",
			method:   http.MethodPatch,
			url:      fmt.Sprintf("/stamps/dilute/%s/18", batchOkStr),
			respCode: http.StatusAccepted,
			resp: &api.PostageCreateResponse{
				BatchID: batchOk,
				TxHash:  txHash.String(),
			},
		},
	}

	failure := []operation{
		{
			name:     "create batch not ok",
			method:   http.MethodPost,
			url:      "/stamps/1000/24?label=test",
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
			op1 := op1
			op2 := op2
			t.Run(op1.name+"-"+op2.name, func(t *testing.T) {
				t.Parallel()

				wait, done := make(chan struct{}), make(chan struct{})
				contract := contractMock.New(
					contractMock.WithCreateBatchFunc(func(ctx context.Context, ib *big.Int, d uint8, i bool, l string) (common.Hash, []byte, error) {
						<-wait
						return txHash, batchOk, nil
					}),
					contractMock.WithTopUpBatchFunc(func(ctx context.Context, id []byte, ib *big.Int) (common.Hash, error) {
						<-wait
						return txHash, nil
					}),
					contractMock.WithDiluteBatchFunc(func(ctx context.Context, id []byte, newDepth uint8) (common.Hash, error) {
						<-wait
						return txHash, nil
					}),
				)

				ts, _, _, _ := newTestServer(t, testServerOptions{
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

//nolint:tparallel
func Test_postageCreateHandler_invalidInputs(t *testing.T) {
	t.Parallel()

	client, _, _, _ := newTestServer(t, testServerOptions{})

	tests := []struct {
		name   string
		amount string
		depth  string
		want   jsonhttp.StatusResponse
	}{{
		name:   "amount - invalid value",
		amount: "a",
		depth:  "1",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "amount",
					Error: "invalid value",
				},
			},
		},
	}, {
		name:   "depth - invalid value",
		amount: "1",
		depth:  "a",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "depth",
					Error: strconv.ErrSyntax.Error(),
				},
			},
		},
	}}

	//nolint:paralleltest
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			jsonhttptest.Request(t, client, http.MethodPost, "/stamps/"+tc.amount+"/"+tc.depth, tc.want.Code,
				jsonhttptest.WithExpectedJSONResponse(tc.want),
			)
		})
	}
}

func Test_postageGetStampBucketsHandler_invalidInputs(t *testing.T) {
	t.Parallel()

	client, _, _, _ := newTestServer(t, testServerOptions{})

	tests := []struct {
		name    string
		batchID string
		want    jsonhttp.StatusResponse
	}{{
		name:    "batch_id - odd hex string",
		batchID: "123",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "batch_id",
					Error: api.ErrHexLength.Error(),
				},
			},
		},
	}, {
		name:    "batch_id - invalid hex character",
		batchID: "123G",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "batch_id",
					Error: api.HexInvalidByteError('G').Error(),
				},
			},
		},
	}, {
		name:    "batch_id - invalid length",
		batchID: "1234",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "batch_id",
					Error: "want len:32",
				},
			},
		},
	}}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			jsonhttptest.Request(t, client, http.MethodGet, "/stamps/"+tc.batchID+"/buckets", tc.want.Code,
				jsonhttptest.WithExpectedJSONResponse(tc.want),
			)
		})
	}
}

func Test_postageGetStampHandler_invalidInputs(t *testing.T) {
	t.Parallel()

	client, _, _, _ := newTestServer(t, testServerOptions{})

	tests := []struct {
		name    string
		batchID string
		want    jsonhttp.StatusResponse
	}{{
		name:    "batch_id - odd hex string",
		batchID: "123",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "batch_id",
					Error: api.ErrHexLength.Error(),
				},
			},
		},
	}, {
		name:    "batch_id - invalid hex character",
		batchID: "123G",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "batch_id",
					Error: api.HexInvalidByteError('G').Error(),
				},
			},
		},
	}, {
		name:    "batch_id - invalid length",
		batchID: "1234",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "batch_id",
					Error: "want len:32",
				},
			},
		},
	}}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			jsonhttptest.Request(t, client, http.MethodGet, "/stamps/"+tc.batchID, tc.want.Code,
				jsonhttptest.WithExpectedJSONResponse(tc.want),
			)
		})
	}
}

//nolint:tparallel
func Test_postageTopUpHandler_invalidInputs(t *testing.T) {
	t.Parallel()

	client, _, _, _ := newTestServer(t, testServerOptions{})

	tests := []struct {
		name    string
		batchID string
		amount  string
		want    jsonhttp.StatusResponse
	}{{
		name:    "batch_id - odd hex string",
		batchID: "123",
		amount:  "1",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "batch_id",
					Error: api.ErrHexLength.Error(),
				},
			},
		},
	}, {
		name:    "batch_id - invalid hex character",
		batchID: "123G",
		amount:  "1",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "batch_id",
					Error: api.HexInvalidByteError('G').Error(),
				},
			},
		},
	}, {
		name:    "batch_id - invalid length",
		batchID: "1234",
		amount:  "1",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "batch_id",
					Error: "want len:32",
				},
			},
		},
	}, {
		name:    "amount - invalid value",
		batchID: hex.EncodeToString([]byte{31: 0}),
		amount:  "a",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "amount",
					Error: "invalid value",
				},
			},
		},
	}}

	//nolint:paralleltest
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			jsonhttptest.Request(t, client, http.MethodPatch, "/stamps/topup/"+tc.batchID+"/"+tc.amount, tc.want.Code,
				jsonhttptest.WithExpectedJSONResponse(tc.want),
			)
		})
	}
}

//nolint:tparallel
func Test_postageDiluteHandler_invalidInputs(t *testing.T) {
	t.Parallel()

	client, _, _, _ := newTestServer(t, testServerOptions{})

	tests := []struct {
		name    string
		batchID string
		depth   string
		want    jsonhttp.StatusResponse
	}{{
		name:    "batch_id - odd hex string",
		batchID: "123",
		depth:   "1",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "batch_id",
					Error: api.ErrHexLength.Error(),
				},
			},
		},
	}, {
		name:    "batch_id - invalid hex character",
		batchID: "123G",
		depth:   "1",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "batch_id",
					Error: api.HexInvalidByteError('G').Error(),
				},
			},
		},
	}, {
		name:    "batch_id - invalid length",
		batchID: "1234",
		depth:   "1",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "batch_id",
					Error: "want len:32",
				},
			},
		},
	}, {
		name:    "depth - invalid syntax",
		batchID: hex.EncodeToString([]byte{31: 0}),
		depth:   "a",
		want: jsonhttp.StatusResponse{
			Code:    http.StatusBadRequest,
			Message: "invalid path params",
			Reasons: []jsonhttp.Reason{
				{
					Field: "depth",
					Error: strconv.ErrSyntax.Error(),
				},
			},
		},
	}}

	//nolint:paralleltest
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			jsonhttptest.Request(t, client, http.MethodPatch, "/stamps/dilute/"+tc.batchID+"/"+tc.depth, tc.want.Code,
				jsonhttptest.WithExpectedJSONResponse(tc.want),
			)
		})
	}
}
