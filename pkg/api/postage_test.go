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

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/bigint"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/postage"
	mockpost "github.com/ethersphere/bee/pkg/postage/mock"
	"github.com/ethersphere/bee/pkg/postage/postagecontract"
	contractMock "github.com/ethersphere/bee/pkg/postage/postagecontract/mock"
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
		client, _, _ := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, client, http.MethodPost, createBatch(initialBalance, depth, label), http.StatusCreated,
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
		client, _, _ := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, client, http.MethodPost, createBatch(initialBalance, depth, label), http.StatusCreated,
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
		client, _, _ := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, client, http.MethodPost, createBatch(initialBalance, depth, label), http.StatusInternalServerError,
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
		client, _, _ := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, client, http.MethodPost, createBatch(initialBalance, depth, label), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "out of funds",
			}),
		)
	})

	t.Run("invalid depth", func(t *testing.T) {
		client, _, _ := newTestServer(t, testServerOptions{})

		jsonhttptest.Request(t, client, http.MethodPost, "/stamps/1000/ab", http.StatusBadRequest,
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
		client, _, _ := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, client, http.MethodPost, "/stamps/1000/9", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid depth",
			}),
		)
	})

	t.Run("invalid balance", func(t *testing.T) {
		client, _, _ := newTestServer(t, testServerOptions{})

		jsonhttptest.Request(t, client, http.MethodPost, "/stamps/abcd/2", http.StatusBadRequest,
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
		client, _, _ := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, client, http.MethodPost, "/stamps/1000/24", http.StatusCreated,
			jsonhttptest.WithRequestHeader("Immutable", "true"),
			jsonhttptest.WithExpectedJSONResponse(&api.PostageCreateResponse{
				BatchID: batchID,
			}),
		)

		if !immutable {
			t.Fatalf("want true, got %v", immutable)
		}

	})
}

func TestPostageGetStamps(t *testing.T) {
	si := postage.NewStampIssuer("", "", batchOk, big.NewInt(3), 11, 10, 1000, true)
	mp := mockpost.New(mockpost.WithIssuer(si))
	client, _, _ := newTestServer(t, testServerOptions{Post: mp})

	jsonhttptest.Request(t, client, http.MethodGet, "/stamps", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(&api.PostageStampsResponse{
			Stamps: []api.PostageStampResponse{
				{
					BatchID:       batchOk,
					Utilization:   si.Utilization(),
					Usable:        true,
					Label:         si.Label(),
					Depth:         si.Depth(),
					Amount:        bigint.Wrap(si.Amount()),
					BucketDepth:   si.BucketDepth(),
					BlockNumber:   si.BlockNumber(),
					ImmutableFlag: si.ImmutableFlag(),
				},
			},
		}),
	)
}

func TestPostageGetStamp(t *testing.T) {
	si := postage.NewStampIssuer("", "", batchOk, big.NewInt(3), 11, 10, 1000, true)
	mp := mockpost.New(mockpost.WithIssuer(si))
	client, _, _ := newTestServer(t, testServerOptions{Post: mp})

	t.Run("ok", func(t *testing.T) {
		jsonhttptest.Request(t, client, http.MethodGet, "/stamps/"+batchOkStr, http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&api.PostageStampResponse{
				BatchID:       batchOk,
				Utilization:   si.Utilization(),
				Usable:        true,
				Label:         si.Label(),
				Depth:         si.Depth(),
				Amount:        bigint.Wrap(si.Amount()),
				BucketDepth:   si.BucketDepth(),
				BlockNumber:   si.BlockNumber(),
				ImmutableFlag: si.ImmutableFlag(),
			}),
		)
	})
	t.Run("ok", func(t *testing.T) {
		badBatch := []byte{0, 1, 2}

		jsonhttptest.Request(t, client, http.MethodGet, "/stamps/"+hex.EncodeToString(badBatch), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid batchID",
			}),
		)
	})
	t.Run("ok", func(t *testing.T) {
		badBatch := []byte{0, 1, 2, 4}

		jsonhttptest.Request(t, client, http.MethodGet, "/stamps/"+hex.EncodeToString(badBatch), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid batchID",
			}),
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
		client, _, _ := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, client, http.MethodPatch, topupBatch(batchOkStr, topupAmount), http.StatusAccepted,
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
		client, _, _ := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, client, http.MethodPatch, topupBatch(batchOkStr, topupAmount), http.StatusAccepted,
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
		client, _, _ := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, client, http.MethodPatch, topupBatch(batchOkStr, topupAmount), http.StatusInternalServerError,
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
		client, _, _ := newTestServer(t, testServerOptions{
			PostageContract: contract,
		})

		jsonhttptest.Request(t, client, http.MethodPatch, topupBatch(batchOkStr, topupAmount), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "out of funds",
			}),
		)
	})

	t.Run("invalid batch id", func(t *testing.T) {
		client, _, _ := newTestServer(t, testServerOptions{})

		jsonhttptest.Request(t, client, http.MethodPatch, "/stamps/topup/abcd/2", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid batchID",
			}),
		)
	})

	t.Run("invalid amount", func(t *testing.T) {
		client, _, _ := newTestServer(t, testServerOptions{})

		wrongURL := fmt.Sprintf("/stamps/topup/%s/amount", batchOkStr)

		jsonhttptest.Request(t, client, http.MethodPatch, wrongURL, http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid postage amount",
			}),
		)
	})
}
