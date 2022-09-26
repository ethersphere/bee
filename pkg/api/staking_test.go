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
	minStakedAmount := big.NewInt(1)
	depositStake := func(address string, amount string) string {
		return fmt.Sprintf("/staking/deposit/%s/%s", address, amount)
	}

	t.Run("ok", func(t *testing.T) {
		contract := stakingContractMock.New(
			stakingContractMock.WithDepositStake(func(ctx context.Context, stakedAmount *big.Int, overlay []byte) error {
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
			stakingContractMock.WithDepositStake(func(ctx context.Context, stakedAmount *big.Int, overlay []byte) error {
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
			stakingContractMock.WithDepositStake(func(ctx context.Context, stakedAmount *big.Int, overlay []byte) error {
				return stakingcontract.ErrInsufficientFunds
			}),
		)
		ts, _, _, _ := newTestServer(t, testServerOptions{DebugAPI: true, StakingContract: contract})
		jsonhttptest.Request(t, ts, http.MethodPost, depositStake(addr.String(), minStake), http.StatusBadRequest)
		jsonhttptest.WithExpectedJSONResponse(&jsonhttp.StatusResponse{Code: http.StatusBadRequest, Message: "out of funds"})
	})

	t.Run("internal error", func(t *testing.T) {
		contract := stakingContractMock.New(
			stakingContractMock.WithDepositStake(func(ctx context.Context, stakedAmount *big.Int, overlay []byte) error {
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
