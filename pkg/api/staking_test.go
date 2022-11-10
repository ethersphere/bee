// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	"fmt"
	"github.com/ethersphere/bee/pkg/bigint"
	"math/big"
	"net/http"
	"testing"

	"github.com/ethersphere/bee/pkg/api"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/storageincentives/staking"
	stakingContractMock "github.com/ethersphere/bee/pkg/storageincentives/staking/mock"
)

func TestDepositStake(t *testing.T) {
	t.Parallel()

	minStake := big.NewInt(100000000000000000).String()
	depositStake := func(amount string) string {
		return fmt.Sprintf("/stake/%s", amount)
	}

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		contract := stakingContractMock.New(
			stakingContractMock.WithDepositStake(func(ctx context.Context, stakedAmount *big.Int) error {
				return nil
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true, StakingContract: contract})
		jsonhttptest.Request(t, ts, http.MethodPost, depositStake(minStake), http.StatusOK)
	})

	t.Run("with invalid stake amount", func(t *testing.T) {
		t.Parallel()

		invalidMinStake := big.NewInt(0).String()
		contract := stakingContractMock.New(
			stakingContractMock.WithDepositStake(func(ctx context.Context, stakedAmount *big.Int) error {
				return staking.ErrInsufficientStakeAmount
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true, StakingContract: contract})
		jsonhttptest.Request(t, ts, http.MethodPost, depositStake(invalidMinStake), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{Code: http.StatusBadRequest, Message: "insufficient stake amount"}))
	})

	t.Run("out of funds", func(t *testing.T) {
		t.Parallel()

		contract := stakingContractMock.New(
			stakingContractMock.WithDepositStake(func(ctx context.Context, stakedAmount *big.Int) error {
				return staking.ErrInsufficientFunds
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true, StakingContract: contract})
		jsonhttptest.Request(t, ts, http.MethodPost, depositStake(minStake), http.StatusBadRequest)
		jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{Code: http.StatusBadRequest, Message: "out of funds"})
	})

	t.Run("internal error", func(t *testing.T) {
		t.Parallel()

		contract := stakingContractMock.New(
			stakingContractMock.WithDepositStake(func(ctx context.Context, stakedAmount *big.Int) error {
				return fmt.Errorf("some error")
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true, StakingContract: contract})
		jsonhttptest.Request(t, ts, http.MethodPost, depositStake(minStake), http.StatusInternalServerError)
		jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{Code: http.StatusInternalServerError, Message: "cannot stake"})
	})

	t.Run("gas limit header", func(t *testing.T) {
		t.Parallel()

		contract := stakingContractMock.New(
			stakingContractMock.WithDepositStake(func(ctx context.Context, stakedAmount *big.Int) error {
				gasLimit := sctx.GetGasLimit(ctx)
				if gasLimit != 2000000 {
					t.Fatalf("want 2000000, got %d", gasLimit)
				}
				return nil
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{
			DebugAPI:        true,
			StakingContract: contract,
		})

		jsonhttptest.Request(t, ts, http.MethodPost, depositStake(minStake), http.StatusOK,
			jsonhttptest.WithRequestHeader("Gas-Limit", "2000000"),
		)
	})
}

func TestGetStake(t *testing.T) {
	t.Parallel()

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		contract := stakingContractMock.New(
			stakingContractMock.WithGetStake(func(ctx context.Context) (*big.Int, error) {
				return big.NewInt(1), nil
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true, StakingContract: contract})
		jsonhttptest.Request(t, ts, http.MethodGet, "/stake", http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&api.GetStakeResponse{StakedAmount: &bigint.BigInt{big.NewInt(1)}}))
	})

	t.Run("with error", func(t *testing.T) {
		t.Parallel()

		contractWithError := stakingContractMock.New(
			stakingContractMock.WithGetStake(func(ctx context.Context) (*big.Int, error) {
				return big.NewInt(0), fmt.Errorf("get stake failed")
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true, StakingContract: contractWithError})
		jsonhttptest.Request(t, ts, http.MethodGet, "/stake", http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{Code: http.StatusInternalServerError, Message: "get staked amount failed"}))
	})
}

func Test_stakingDepositHandler_invalidInputs(t *testing.T) {
	t.Parallel()

	client, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true})

	tests := []struct {
		name   string
		amount string
		want   jsonhttp.StatusResponse
	}{{
		name:   "amount - invalid value",
		amount: "a",
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

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			jsonhttptest.Request(t, client, http.MethodPost, "/stake/"+tc.amount, tc.want.Code,
				jsonhttptest.WithExpectedJSONResponse(tc.want),
			)
		})
	}
}
