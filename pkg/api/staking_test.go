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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/jsonhttp/jsonhttptest"
	"github.com/ethersphere/bee/pkg/staking/stakingcontract"
	stakingContractMock "github.com/ethersphere/bee/pkg/staking/stakingcontract/mock"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestDepositStake(t *testing.T) {
	k, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	addr, err := crypto.NewOverlayAddress(k.PublicKey, 1, common.HexToHash("0x1").Bytes())
	if err != nil {
		t.Fatal(err)
	}
	minStake := big.NewInt(1).String()
	minStakedAmount := stakingcontract.MinimumStakeAmount
	depositStake := func(address string, amount string) string {
		return fmt.Sprintf("/stake/deposit/%s/%s", address, amount)
	}

	t.Run("ok", func(t *testing.T) {
		contract := stakingContractMock.New(
			stakingContractMock.WithDepositStake(func(ctx context.Context, stakedAmount big.Int, overlay swarm.Address) error {
				if stakedAmount.Cmp(minStakedAmount) == -1 {
					return stakingcontract.ErrInvalidStakeAmount
				}
				return nil
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true, StakingContract: contract})
		jsonhttptest.Request(t, ts, http.MethodPost, depositStake(addr.String(), minStake), http.StatusOK)
	})

	t.Run("with invalid stake amount", func(t *testing.T) {
		invalidMinStake := big.NewInt(0).String()
		invalidMinStakedAmount := big.NewInt(1)
		contract := stakingContractMock.New(
			stakingContractMock.WithDepositStake(func(ctx context.Context, stakedAmount big.Int, overlay swarm.Address) error {
				if stakedAmount.Cmp(invalidMinStakedAmount) == -1 {
					return stakingcontract.ErrInvalidStakeAmount
				}
				return nil
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true, StakingContract: contract})
		jsonhttptest.Request(t, ts, http.MethodPost, depositStake(addr.String(), invalidMinStake), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{Code: http.StatusBadRequest, Message: "minimum 1 BZZ required for staking"}))
	})

	t.Run("with invalid address", func(t *testing.T) {
		invalidMinStake := big.NewInt(0).String()
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true})
		jsonhttptest.Request(t, ts, http.MethodPost, depositStake("invalid address", invalidMinStake), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{Code: http.StatusBadRequest, Message: "invalid address"}))
	})

	t.Run("out of funds", func(t *testing.T) {
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
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true})
		jsonhttptest.Request(t, ts, http.MethodPost, depositStake(addr.String(), "abc"), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{Code: http.StatusBadRequest, Message: "invalid staking amount"}))
	})
}

func TestGetStake(t *testing.T) {
	k, err := crypto.GenerateSecp256k1Key()
	if err != nil {
		t.Fatal(err)
	}
	addr, err := crypto.NewOverlayAddress(k.PublicKey, 1, common.HexToHash("0x1").Bytes())
	if err != nil {
		t.Fatal(err)
	}
	getStake := func(address string) string {
		return fmt.Sprintf("/stake/%s", address)
	}

	t.Run("ok", func(t *testing.T) {
		contract := stakingContractMock.New(
			stakingContractMock.WithGetStake(func(ctx context.Context, overlay swarm.Address) (big.Int, error) {
				return *big.NewInt(1), nil
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true, StakingContract: contract})
		jsonhttptest.Request(t, ts, http.MethodGet, getStake(addr.String()), http.StatusOK)
	})

	t.Run("with invalid address", func(t *testing.T) {
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true})
		jsonhttptest.Request(t, ts, http.MethodGet, getStake("invalid address"), http.StatusBadRequest,
			jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{Code: http.StatusBadRequest, Message: "invalid address"}))
	})

	t.Run("with error", func(t *testing.T) {
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
