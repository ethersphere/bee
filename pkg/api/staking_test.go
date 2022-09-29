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
	"github.com/ethersphere/bee/pkg/staking/stakingcontract"
	stakingContractMock "github.com/ethersphere/bee/pkg/staking/stakingcontract/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestDepositStake(t *testing.T) {
	t.Parallel()

	addr := swarm.MustParseHexAddress("f30c0aa7e9e2a0ef4c9b1b750ebfeaeb7c7c24da700bb089da19a46e3677824b")

	minStake := big.NewInt(1).String()
	depositStake := func(address string, amount string) string {
		return fmt.Sprintf("/stake/deposit/%s/%s", address, amount)
	}

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		contract := stakingContractMock.New(
			stakingContractMock.WithDepositStake(func(ctx context.Context, stakedAmount big.Int, overlay swarm.Address) error {
				return nil
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true, StakingContract: contract})
		jsonhttptest.Request(t, ts, http.MethodPost, depositStake(addr.String(), minStake), http.StatusOK)
	})

	t.Run("with invalid stake amount", func(t *testing.T) {
		t.Parallel()

		invalidMinStake := big.NewInt(0).String()
		contract := stakingContractMock.New(
			stakingContractMock.WithDepositStake(func(ctx context.Context, stakedAmount big.Int, overlay swarm.Address) error {
				return stakingcontract.ErrInsufficentStakeAmount
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true, StakingContract: contract})
		jsonhttptest.Request(t, ts, http.MethodPost, depositStake(addr.String(), invalidMinStake), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{Code: http.StatusBadRequest, Message: "minimum 1 BZZ required for staking"}))
	})

	t.Run("with invalid address", func(t *testing.T) {
		t.Parallel()

		invalidMinStake := big.NewInt(0).String()
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true})
		jsonhttptest.Request(t, ts, http.MethodPost, depositStake("invalid address", invalidMinStake), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{Code: http.StatusBadRequest, Message: "invalid address"}))
	})

	t.Run("out of funds", func(t *testing.T) {
		t.Parallel()

		contract := stakingContractMock.New(
			stakingContractMock.WithDepositStake(func(ctx context.Context, stakedAmount big.Int, overlay swarm.Address) error {
				return stakingcontract.ErrInsufficientFunds
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true, StakingContract: contract})
		jsonhttptest.Request(t, ts, http.MethodPost, depositStake(addr.String(), minStake), http.StatusBadRequest)
		jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{Code: http.StatusBadRequest, Message: "out of funds"})
	})

	t.Run("internal error", func(t *testing.T) {
		t.Parallel()

		contract := stakingContractMock.New(
			stakingContractMock.WithDepositStake(func(ctx context.Context, stakedAmount big.Int, overlay swarm.Address) error {
				return fmt.Errorf("some error")
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true, StakingContract: contract})
		jsonhttptest.Request(t, ts, http.MethodPost, depositStake(addr.String(), minStake), http.StatusInternalServerError)
		jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{Code: http.StatusInternalServerError, Message: "cannot stake"})
	})

	t.Run("with invalid amount", func(t *testing.T) {
		t.Parallel()

		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true})
		jsonhttptest.Request(t, ts, http.MethodPost, depositStake(addr.String(), "abc"), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{Code: http.StatusBadRequest, Message: "invalid staking amount"}))
	})
}

func TestGetStake(t *testing.T) {
	t.Parallel()

	addr := swarm.MustParseHexAddress("f30c0aa7e9e2a0ef4c9b1b750ebfeaeb7c7c24da700bb089da19a46e3677824b")

	getStake := func(address string) string {
		return fmt.Sprintf("/stake/%s", address)
	}

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		contract := stakingContractMock.New(
			stakingContractMock.WithGetStake(func(ctx context.Context, overlay swarm.Address) (big.Int, error) {
				return *big.NewInt(1), nil
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true, StakingContract: contract})
		jsonhttptest.Request(t, ts, http.MethodGet, getStake(addr.String()), http.StatusOK,
			jsonhttptest.WithExpectedJSONResponse(&api.GetStakeResponse{StakedAmount: big.NewInt(1)}))
	})

	t.Run("with invalid address", func(t *testing.T) {
		t.Parallel()

		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true})
		jsonhttptest.Request(t, ts, http.MethodGet, getStake("invalid address"), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{Code: http.StatusBadRequest, Message: "invalid address"}))
	})

	t.Run("with error", func(t *testing.T) {
		t.Parallel()

		contractWithError := stakingContractMock.New(
			stakingContractMock.WithGetStake(func(ctx context.Context, overlay swarm.Address) (big.Int, error) {
				return *big.NewInt(0), stakingcontract.ErrGetStakeFailed
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true, StakingContract: contractWithError})
		jsonhttptest.Request(t, ts, http.MethodGet, getStake(addr.String()), http.StatusInternalServerError,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{Code: http.StatusInternalServerError, Message: "get staked amount failed"}))
	})
}
