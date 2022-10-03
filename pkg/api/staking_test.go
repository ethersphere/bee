// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api_test

import (
	"context"
	"fmt"
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
		return fmt.Sprintf("/stake/deposit/%s", amount)
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
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{Code: http.StatusBadRequest, Message: "minimum 100000000000000000 BZZ required for staking"}))
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

	t.Run("with invalid amount", func(t *testing.T) {
		t.Parallel()

		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true})
		jsonhttptest.Request(t, ts, http.MethodPost, depositStake("abc"), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{Code: http.StatusBadRequest, Message: "invalid staking amount"}))
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
			jsonhttptest.WithExpectedJSONResponse(&api.GetStakeResponse{StakedAmount: big.NewInt(1)}))
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
