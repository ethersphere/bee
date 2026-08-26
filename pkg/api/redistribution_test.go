// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"bytes"
	"context"
	"math/big"
	"net/http"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/ethersphere/bee/v2/pkg/api"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp/jsonhttptest"
	statestore "github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/storageincentives"
	"github.com/ethersphere/bee/v2/pkg/transaction/backendmock"
	"github.com/ethersphere/bee/v2/pkg/transaction/mock"
)

func TestRedistributionStatus(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		store := statestore.NewStateStore()
		err := store.Put("redistribution_state", storageincentives.Status{
			Phase: storageincentives.PhaseType(1),
			Round: 1,
			Block: 12,
		})
		if err != nil {
			t.Errorf("redistribution put state: %v", err)
		}
		srv, _, _, _ := newTestServer(t, testServerOptions{
			StateStorer: store,
			TransactionOpts: []mock.Option{
				mock.WithTransactionFeeFunc(func(ctx context.Context, txHash common.Hash) (*big.Int, error) {
					return big.NewInt(1000), nil
				}),
			},
			BackendOpts: []backendmock.Option{
				backendmock.WithBalanceAt(func(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error) {
					return big.NewInt(100000000), nil
				}),
				backendmock.WithSuggestedFeeAndTipFunc(func(ctx context.Context, gasPrice *big.Int, boostPercent int) (*big.Int, *big.Int, error) {
					return big.NewInt(1), big.NewInt(2), nil
				}),
			},
		})
		var got api.RedistributionStatusResponse
		jsonhttptest.Request(t, srv, http.MethodGet, "/redistributionstate", http.StatusOK,
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "application/json; charset=utf-8"),
			jsonhttptest.WithUnmarshalJSONResponse(&got),
		)
		if !got.Enabled {
			t.Fatal("expected redistribution to be enabled by default")
		}
	})

	t.Run("bad request", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _ := newTestServer(t, testServerOptions{
			BeeMode:     api.LightMode,
			StateStorer: statestore.NewStateStore(),
			TransactionOpts: []mock.Option{
				mock.WithTransactionFeeFunc(func(ctx context.Context, txHash common.Hash) (*big.Int, error) {
					return big.NewInt(1000), nil
				}),
			},
		})
		jsonhttptest.Request(t, srv, http.MethodGet, "/redistributionstate", http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: api.ErrOperationSupportedOnlyInFullMode.Error(),
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("forbidden when agent missing", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _ := newTestServer(t, testServerOptions{
			RedistributionAgentDisabled: true,
		})
		jsonhttptest.Request(t, srv, http.MethodGet, "/redistributionstate", http.StatusForbidden,
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "Storage incentives are disabled. This endpoint is unavailable.",
				Code:    http.StatusForbidden,
			}),
		)
	})
}

func redistributionTestOpts(t *testing.T) testServerOptions {
	t.Helper()

	store := statestore.NewStateStore()
	if err := store.Put("redistribution_state", storageincentives.Status{
		Phase: storageincentives.PhaseType(1),
		Round: 1,
		Block: 12,
	}); err != nil {
		t.Fatal(err)
	}

	return testServerOptions{
		StateStorer: store,
		TransactionOpts: []mock.Option{
			mock.WithTransactionFeeFunc(func(ctx context.Context, txHash common.Hash) (*big.Int, error) {
				return big.NewInt(1000), nil
			}),
		},
		BackendOpts: []backendmock.Option{
			backendmock.WithBalanceAt(func(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error) {
				return big.NewInt(100000000), nil
			}),
			backendmock.WithSuggestedFeeAndTipFunc(func(ctx context.Context, gasPrice *big.Int, boostPercent int) (*big.Int, *big.Int, error) {
				return big.NewInt(1), big.NewInt(2), nil
			}),
		},
	}
}

func TestRedistributionToggle(t *testing.T) {
	t.Parallel()

	t.Run("put false then true", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _ := newTestServer(t, redistributionTestOpts(t))

		jsonhttptest.Request(t, srv, http.MethodPut, "/redistribution", http.StatusOK,
			jsonhttptest.WithJSONRequestBody(map[string]any{"enabled": false}),
			jsonhttptest.WithExpectedJSONResponse(api.RedistributionToggleResponse{Enabled: false}),
		)

		var got api.RedistributionStatusResponse
		jsonhttptest.Request(t, srv, http.MethodGet, "/redistributionstate", http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&got),
		)
		if got.Enabled {
			t.Fatal("expected redistribution to be disabled")
		}

		jsonhttptest.Request(t, srv, http.MethodPut, "/redistribution", http.StatusOK,
			jsonhttptest.WithJSONRequestBody(map[string]any{"enabled": true}),
			jsonhttptest.WithExpectedJSONResponse(api.RedistributionToggleResponse{Enabled: true}),
		)

		jsonhttptest.Request(t, srv, http.MethodGet, "/redistributionstate", http.StatusOK,
			jsonhttptest.WithUnmarshalJSONResponse(&got),
		)
		if !got.Enabled {
			t.Fatal("expected redistribution to be enabled")
		}
	})

	t.Run("missing enabled", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _ := newTestServer(t, redistributionTestOpts(t))
		jsonhttptest.Request(t, srv, http.MethodPut, "/redistribution", http.StatusBadRequest,
			jsonhttptest.WithJSONRequestBody(map[string]any{}),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "enabled is required",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("null enabled", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _ := newTestServer(t, redistributionTestOpts(t))
		jsonhttptest.Request(t, srv, http.MethodPut, "/redistribution", http.StatusBadRequest,
			jsonhttptest.WithJSONRequestBody(map[string]any{"enabled": nil}),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "enabled is required",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("malformed json", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _ := newTestServer(t, redistributionTestOpts(t))
		jsonhttptest.Request(t, srv, http.MethodPut, "/redistribution", http.StatusBadRequest,
			jsonhttptest.WithRequestHeader(api.ContentTypeHeader, "application/json"),
			jsonhttptest.WithRequestBody(bytes.NewReader([]byte("{invalid"))),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "invalid request body",
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("light mode", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _ := newTestServer(t, testServerOptions{
			BeeMode:     api.LightMode,
			StateStorer: statestore.NewStateStore(),
		})
		jsonhttptest.Request(t, srv, http.MethodPut, "/redistribution", http.StatusBadRequest,
			jsonhttptest.WithJSONRequestBody(map[string]any{"enabled": false}),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: api.ErrOperationSupportedOnlyInFullMode.Error(),
				Code:    http.StatusBadRequest,
			}),
		)
	})

	t.Run("forbidden when agent missing", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _ := newTestServer(t, testServerOptions{
			RedistributionAgentDisabled: true,
		})
		jsonhttptest.Request(t, srv, http.MethodPut, "/redistribution", http.StatusForbidden,
			jsonhttptest.WithJSONRequestBody(map[string]any{"enabled": false}),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "Storage incentives are disabled. This endpoint is unavailable.",
				Code:    http.StatusForbidden,
			}),
		)
	})

	t.Run("unavailable when full api disabled", func(t *testing.T) {
		t.Parallel()

		srv, _, _, _ := newTestServer(t, testServerOptions{
			FullAPIDisabled: true,
		})
		jsonhttptest.Request(t, srv, http.MethodPut, "/redistribution", http.StatusServiceUnavailable,
			jsonhttptest.WithJSONRequestBody(map[string]any{"enabled": false}),
			jsonhttptest.WithExpectedJSONResponse(jsonhttp.StatusResponse{
				Message: "Node is syncing. This endpoint is unavailable. Try again later.",
				Code:    http.StatusServiceUnavailable,
			}),
		)
	})
}
