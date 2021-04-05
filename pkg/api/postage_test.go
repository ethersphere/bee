// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/postage/postagecontract"
	contractMock "github.com/ethersphere/bee/pkg/postage/postagecontract/mock"
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
			contractMock.WithCreateBatchFunc(func(ctx context.Context, ib *big.Int, d uint8, l string) ([]byte, error) {
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

		jsonhttptest.Request(t, client, http.MethodPost, createBatch(initialBalance, depth, label), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&api.PostageCreateResponse{
				BatchID: batchID,
			}),
		)
	})

	t.Run("with-error", func(t *testing.T) {
		contract := contractMock.New(
			contractMock.WithCreateBatchFunc(func(ctx context.Context, ib *big.Int, d uint8, l string) ([]byte, error) {
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
			contractMock.WithCreateBatchFunc(func(ctx context.Context, ib *big.Int, d uint8, l string) ([]byte, error) {
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
			contractMock.WithCreateBatchFunc(func(ctx context.Context, ib *big.Int, d uint8, l string) ([]byte, error) {
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
}

func TestPostageGetStamps(t *testing.T) {
	batchID := make([]byte, 32) // this will break once the fallback batchid is gone
	client, _, _ := newTestServer(t, testServerOptions{})

	jsonhttptest.Request(t, client, http.MethodGet, "/stamps", http.StatusOK,
		jsonhttptest.WithExpectedJSONResponse(&api.PostageStampsResponse{
			Stamps: []api.PostageStampResponse{
				api.PostageStampResponse{
					BatchID:     batchID,
					Utilization: 0,
				},
			},
		}),
	)
}

func TestPostageGetStamp(t *testing.T) {
	batchID := make([]byte, 32) // this will break once the fallback batchid is gone

	t.Run("ok", func(t *testing.T) {
		client, _, _ := newTestServer(t, testServerOptions{})

		jsonhttptest.Request(t, client, http.MethodGet, "/stamps/"+hex.EncodeToString(batchID), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&api.PostageStampResponse{
				BatchID:     batchID,
				Utilization: 0,
			}),
		)
	})
	t.Run("ok", func(t *testing.T) {
		client, _, _ := newTestServer(t, testServerOptions{})
		badBatch := []byte{0, 1, 2}

		jsonhttptest.Request(t, client, http.MethodGet, "/stamps/"+hex.EncodeToString(badBatch), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid batchID",
			}),
		)
	})
	t.Run("ok", func(t *testing.T) {
		client, _, _ := newTestServer(t, testServerOptions{})
		badBatch := []byte{0, 1, 2, 4}

		jsonhttptest.Request(t, client, http.MethodGet, "/stamps/"+hex.EncodeToString(badBatch), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{
				Code:    http.StatusBadRequest,
				Message: "invalid batchID",
			}),
		)
	})
}
