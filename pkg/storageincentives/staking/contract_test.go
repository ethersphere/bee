// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package staking_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	chaincfg "github.com/ethersphere/bee/v2/pkg/config"
	"github.com/ethersphere/bee/v2/pkg/storageincentives/staking"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	transactionMock "github.com/ethersphere/bee/v2/pkg/transaction/mock"
	"github.com/ethersphere/bee/v2/pkg/util/abiutil"
)

var stakingContractABI = abiutil.MustParseABI(chaincfg.Testnet.StakingABI)

const stakingHeight = uint8(0)

func TestIsOverlayFrozen(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	owner := common.HexToAddress("abcd")
	stakingContractAddress := common.HexToAddress("ffff")
	bzzTokenAddress := common.HexToAddress("eeee")
	nonce := common.BytesToHash(make([]byte, 32))

	height := 100
	frozenHeight := big.NewInt(int64(height))

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingContractAddress {
						return frozenHeight.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		frozen, err := contract.IsOverlayFrozen(ctx, uint64(height-1))
		if err != nil {
			t.Fatal(err)
		}

		if !frozen {
			t.Fatalf("expected owner to be frozen")
		}

		frozen, err = contract.IsOverlayFrozen(ctx, uint64(height+1))
		if err != nil {
			t.Fatal(err)
		}

		if frozen {
			t.Fatalf("expected owner to not be frozen")
		}

	})
}

func TestDepositStake(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	owner := common.HexToAddress("abcd")
	stakingContractAddress := common.HexToAddress("ffff")
	bzzTokenAddress := common.HexToAddress("eeee")
	nonce := common.BytesToHash(make([]byte, 32))
	txHashDeposited := common.HexToHash("c3a7")
	stakedAmount := big.NewInt(100000000000000000)
	txHashApprove := common.HexToHash("abb0")

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		totalAmount := big.NewInt(100000000000000000)
		prevStake := big.NewInt(0)
		expectedCallData, err := stakingContractABI.Pack("manageStake", nonce, stakedAmount, stakingHeight)
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == bzzTokenAddress {
						return txHashApprove, nil
					}
					if *request.To == stakingContractAddress {
						if !bytes.Equal(expectedCallData[:80], request.Data[:80]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return txHashDeposited, nil
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashDeposited {
						return &types.Receipt{
							Status: 1,
						}, nil
					}
					if txHash == txHashApprove {
						return &types.Receipt{
							Status: 1,
						}, nil
					}
					return nil, errors.New("unknown tx hash")
				}),
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == bzzTokenAddress {
						return totalAmount.FillBytes(make([]byte, 32)), nil
					}
					if *request.To == stakingContractAddress {
						return getPotentialStakeResponse(t, prevStake), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err = contract.DepositStake(ctx, stakedAmount)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("ok with addon stake", func(t *testing.T) {
		t.Parallel()

		totalAmount := big.NewInt(100000000000000000)
		prevStake := big.NewInt(2)
		expectedCallData, err := stakingContractABI.Pack("manageStake", nonce, big.NewInt(100000000000000000), stakingHeight)
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == bzzTokenAddress {
						return txHashApprove, nil
					}
					if *request.To == stakingContractAddress {
						if !bytes.Equal(expectedCallData[:80], request.Data[:80]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return txHashDeposited, nil
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashDeposited {
						return &types.Receipt{
							Status: 1,
						}, nil
					}
					if txHash == txHashApprove {
						return &types.Receipt{
							Status: 1,
						}, nil
					}
					return nil, errors.New("unknown tx hash")
				}),
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == bzzTokenAddress {
						return totalAmount.FillBytes(make([]byte, 32)), nil
					}
					if *request.To == stakingContractAddress {
						return getPotentialStakeResponse(t, prevStake), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err = contract.DepositStake(ctx, stakedAmount)
		if err != nil {
			t.Fatal(err)
		}
		stakedAmount, err := contract.GetPotentialStake(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if stakedAmount.Cmp(big.NewInt(13)) == 0 {
			t.Fatalf("expected %v, got %v", big.NewInt(13), stakedAmount)
		}
	})

	t.Run("insufficient stake amount", func(t *testing.T) {
		t.Parallel()

		totalAmount := big.NewInt(102400)
		prevStake := big.NewInt(0)

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == bzzTokenAddress {
						return totalAmount.FillBytes(make([]byte, 32)), nil
					}
					if *request.To == stakingContractAddress {
						return getPotentialStakeResponse(t, prevStake), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err := contract.DepositStake(ctx, big.NewInt(0))
		if !errors.Is(err, staking.ErrInsufficientStakeAmount) {
			t.Fatal(fmt.Errorf("wanted %w, got %w", staking.ErrInsufficientStakeAmount, err))
		}
	})

	t.Run("insufficient funds", func(t *testing.T) {
		t.Parallel()

		totalAmount := big.NewInt(0)
		prevStake := big.NewInt(0)

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == bzzTokenAddress {
						return totalAmount.FillBytes(make([]byte, 32)), nil
					}
					if *request.To == stakingContractAddress {
						return getPotentialStakeResponse(t, prevStake), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err := contract.DepositStake(ctx, big.NewInt(100000000000000000))
		if !errors.Is(err, staking.ErrInsufficientFunds) {
			t.Fatal(fmt.Errorf("wanted %w, got %w", staking.ErrInsufficientFunds, err))
		}
	})

	t.Run("sufficient stake amount extra height", func(t *testing.T) {
		t.Parallel()

		balance := big.NewInt(0).Mul(staking.MinimumStakeAmount, big.NewInt(2))
		prevStake := staking.MinimumStakeAmount

		expectedCallData, err := stakingContractABI.Pack("manageStake", nonce, staking.MinimumStakeAmount, stakingHeight)
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == bzzTokenAddress {
						return txHashApprove, nil
					}
					if *request.To == stakingContractAddress {
						if !bytes.Equal(expectedCallData[:80], request.Data[:80]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return txHashDeposited, nil
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashDeposited {
						return &types.Receipt{
							Status: 1,
						}, nil
					}
					if txHash == txHashApprove {
						return &types.Receipt{
							Status: 1,
						}, nil
					}
					return nil, errors.New("unknown tx hash")
				}),
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == bzzTokenAddress {
						return balance.FillBytes(make([]byte, 32)), nil
					}
					if *request.To == stakingContractAddress {
						return getPotentialStakeResponse(t, prevStake), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			1,
		)

		_, err = contract.DepositStake(ctx, stakedAmount)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("insufficient stake amount extra height", func(t *testing.T) {
		t.Parallel()

		totalAmount := big.NewInt(0).Mul(staking.MinimumStakeAmount, big.NewInt(10))
		prevStake := big.NewInt(0)

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == bzzTokenAddress {
						return totalAmount.FillBytes(make([]byte, 32)), nil
					}
					if *request.To == stakingContractAddress {
						return getPotentialStakeResponse(t, prevStake), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			1,
		)

		_, err := contract.DepositStake(ctx, stakedAmount)
		if !errors.Is(err, staking.ErrInsufficientStakeAmount) {
			t.Fatal(fmt.Errorf("wanted %w, got %w", staking.ErrInsufficientStakeAmount, err))
		}
	})

	t.Run("send tx failed", func(t *testing.T) {
		t.Parallel()

		totalAmount := big.NewInt(102400)
		prevStake := big.NewInt(0)

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == bzzTokenAddress {
						return common.Hash{}, errors.New("send transaction failed")
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == bzzTokenAddress {
						return totalAmount.FillBytes(make([]byte, 32)), nil
					}
					if *request.To == stakingContractAddress {
						return prevStake.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err := contract.DepositStake(ctx, stakedAmount)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("invalid call data", func(t *testing.T) {
		t.Parallel()

		totalAmount := big.NewInt(102400)
		prevStake := big.NewInt(0)
		expectedStakeAmount := big.NewInt(100)
		expectedCallData, err := staking.Erc20ABI.Pack("approve", stakingContractAddress, expectedStakeAmount)
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == bzzTokenAddress {
						if !bytes.Equal(expectedCallData[:], request.Data[:]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return txHashApprove, nil
					}
					if *request.To == stakingContractAddress {
						return txHashDeposited, nil
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == bzzTokenAddress {
						return totalAmount.FillBytes(make([]byte, 32)), nil
					}
					if *request.To == stakingContractAddress {
						return prevStake.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err = contract.DepositStake(ctx, stakedAmount)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("transaction reverted", func(t *testing.T) {
		t.Parallel()

		totalAmount := big.NewInt(100000000000000000)
		prevStake := big.NewInt(0)
		expectedCallData, err := staking.Erc20ABI.Pack("approve", stakingContractAddress, stakedAmount)
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == bzzTokenAddress {
						return txHashApprove, nil
					}
					if *request.To == stakingContractAddress {
						if !bytes.Equal(expectedCallData[:80], request.Data[:80]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return txHashDeposited, nil
					}
					return txHashDeposited, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashDeposited {
						return &types.Receipt{
							Status: 1,
						}, nil
					}
					if txHash == txHashApprove {
						return &types.Receipt{
							Status: 0,
						}, nil
					}
					return nil, errors.New("unknown tx hash")
				}),
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == bzzTokenAddress {
						return totalAmount.FillBytes(make([]byte, 32)), nil
					}
					if *request.To == stakingContractAddress {
						return getPotentialStakeResponse(t, prevStake), nil

					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err = contract.DepositStake(ctx, stakedAmount)
		if !errors.Is(err, transaction.ErrTransactionReverted) {
			t.Fatalf("expected %v, got %v", transaction.ErrTransactionReverted, err)
		}
	})

	t.Run("transaction error", func(t *testing.T) {
		t.Parallel()

		totalAmount := big.NewInt(102400)
		prevStake := big.NewInt(0)
		expectedCallData, err := staking.Erc20ABI.Pack("approve", stakingContractAddress, stakedAmount)
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == bzzTokenAddress {
						return txHashApprove, nil
					}
					if *request.To == stakingContractAddress {
						if !bytes.Equal(expectedCallData[:80], request.Data[:80]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return txHashDeposited, nil
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashDeposited {
						return &types.Receipt{
							Status: 1,
						}, nil
					}
					if txHash == txHashApprove {
						return nil, fmt.Errorf("unknown error")
					}
					return nil, errors.New("unknown tx hash")
				}),
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == bzzTokenAddress {
						return totalAmount.FillBytes(make([]byte, 32)), nil
					}
					if *request.To == stakingContractAddress {
						return getPotentialStakeResponse(t, prevStake), nil

					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err = contract.DepositStake(ctx, stakedAmount)
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("transaction error in call", func(t *testing.T) {
		t.Parallel()

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingContractAddress {
						return nil, errors.New("some error")
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err := contract.DepositStake(ctx, stakedAmount)
		if err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestChangeHeight(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	owner := common.HexToAddress("abcd")
	stakingContractAddress := common.HexToAddress("ffff")
	bzzTokenAddress := common.HexToAddress("eeee")
	nonce := common.BytesToHash(make([]byte, 32))
	txHashDeposited := common.HexToHash("c3a7")
	stakedAmount := big.NewInt(0)
	txHashApprove := common.HexToHash("abb0")

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		expectedCallData, err := stakingContractABI.Pack("manageStake", nonce, stakedAmount, stakingHeight)
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == stakingContractAddress {
						if !bytes.Equal(expectedCallData[:80], request.Data[:80]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return txHashDeposited, nil
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashDeposited {
						return &types.Receipt{
							Status: 1,
						}, nil
					}
					if txHash == txHashApprove {
						return &types.Receipt{
							Status: 1,
						}, nil
					}
					return nil, errors.New("unknown tx hash")
				}),
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingContractAddress {
						ret := make([]byte, 32)
						ret[1] = stakingHeight
						return ret, nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, updated, err := contract.UpdateHeight(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if updated {
			t.Fatal("expected height not to change")
		}
	})

	t.Run("ok - height increased", func(t *testing.T) {
		t.Parallel()

		var (
			oldHeight uint8 = 0
			newHeight uint8 = 1
		)

		expectedCallData, err := stakingContractABI.Pack("manageStake", nonce, stakedAmount, newHeight)
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == stakingContractAddress {
						if !bytes.Equal(expectedCallData[:80], request.Data[:80]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return txHashDeposited, nil
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashDeposited {
						return &types.Receipt{
							Status: 1,
						}, nil
					}
					if txHash == txHashApprove {
						return &types.Receipt{
							Status: 1,
						}, nil
					}
					return nil, errors.New("unknown tx hash")
				}),
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingContractAddress {
						ret := make([]byte, 32)
						ret[31] = oldHeight
						return ret, nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			newHeight,
		)

		_, updated, err := contract.UpdateHeight(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !updated {
			t.Fatal("expected height to change")
		}
	})

	t.Run("ok - height decreased", func(t *testing.T) {
		t.Parallel()

		var (
			oldHeight uint8 = 1
			newHeight uint8 = 0
		)

		expectedCallData, err := stakingContractABI.Pack("manageStake", nonce, stakedAmount, newHeight)
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == stakingContractAddress {
						if !bytes.Equal(expectedCallData[:80], request.Data[:80]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return txHashDeposited, nil
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashDeposited {
						return &types.Receipt{
							Status: 1,
						}, nil
					}
					if txHash == txHashApprove {
						return &types.Receipt{
							Status: 1,
						}, nil
					}
					return nil, errors.New("unknown tx hash")
				}),
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingContractAddress {
						ret := make([]byte, 32)
						ret[31] = oldHeight
						return ret, nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			newHeight,
		)

		_, updated, err := contract.UpdateHeight(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !updated {
			t.Fatal("expected height to change")
		}
	})

	t.Run("send tx failed", func(t *testing.T) {
		t.Parallel()

		prevStake := big.NewInt(0)

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingContractAddress {
						return prevStake.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err := contract.DepositStake(ctx, stakedAmount)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("transaction error in call", func(t *testing.T) {
		t.Parallel()

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingContractAddress {
						return nil, errors.New("some error")
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, _, err := contract.UpdateHeight(ctx)
		if err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestChangeStakeOverlay(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	owner := common.HexToAddress("abcd")
	stakingContractAddress := common.HexToAddress("ffff")
	bzzTokenAddress := common.HexToAddress("eeee")
	nonce := common.BytesToHash(make([]byte, 32))
	txHashOverlayChanged := common.HexToHash("c3a7")
	stakedAmount := big.NewInt(0)
	txHashApprove := common.HexToHash("abb0")

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		expectedCallData, err := stakingContractABI.Pack("manageStake", nonce, stakedAmount, stakingHeight)
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == stakingContractAddress {
						if !bytes.Equal(expectedCallData[:80], request.Data[:80]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return txHashOverlayChanged, nil
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashOverlayChanged {
						return &types.Receipt{
							Status: 1,
						}, nil
					}
					return nil, errors.New("unknown tx hash")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err = contract.ChangeStakeOverlay(ctx, nonce)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("send tx failed", func(t *testing.T) {
		t.Parallel()

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == stakingContractAddress {
						return common.Hash{}, errors.New("send transaction failed")
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err := contract.ChangeStakeOverlay(ctx, nonce)
		if err == nil || !strings.Contains(err.Error(), "send transaction failed") {
			t.Fatal("expected different error")
		}
	})

	t.Run("invalid call data", func(t *testing.T) {
		t.Parallel()

		expectedCallData, err := stakingContractABI.Pack("manageStake", nonce, stakedAmount, stakingHeight)
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == stakingContractAddress {
						if !bytes.Equal(expectedCallData[:80], request.Data[:80]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return txHashApprove, nil
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		newNonce := make([]byte, 32)
		copy(newNonce, nonce[:])
		newNonce[0]++
		_, err = contract.ChangeStakeOverlay(ctx, common.BytesToHash(newNonce))
		if err == nil || !strings.Contains(err.Error(), "got wrong call data. wanted") {
			t.Fatal("expected different error")
		}
	})

	t.Run("transaction reverted", func(t *testing.T) {
		t.Parallel()

		expectedCallData, err := stakingContractABI.Pack("manageStake", nonce, stakedAmount, stakingHeight)
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == stakingContractAddress {
						if !bytes.Equal(expectedCallData[:80], request.Data[:80]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return txHashOverlayChanged, nil
					}
					return txHashOverlayChanged, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashOverlayChanged {
						return &types.Receipt{
							Status: 0,
						}, nil
					}
					return nil, errors.New("unknown tx hash")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err = contract.ChangeStakeOverlay(ctx, nonce)
		if !errors.Is(err, transaction.ErrTransactionReverted) {
			t.Fatalf("expected %v, got %v", transaction.ErrTransactionReverted, err)
		}
	})

	t.Run("transaction error", func(t *testing.T) {
		t.Parallel()

		expectedCallData, err := stakingContractABI.Pack("manageStake", nonce, stakedAmount, stakingHeight)
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == stakingContractAddress {
						if !bytes.Equal(expectedCallData[:80], request.Data[:80]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return txHashOverlayChanged, nil
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashOverlayChanged {
						return nil, fmt.Errorf("unknown error")
					}
					return nil, errors.New("unknown tx hash")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err = contract.ChangeStakeOverlay(ctx, nonce)
		if err == nil || !strings.Contains(err.Error(), "unknown error") {
			t.Fatal("expected different error")
		}
	})
}

func TestGetCommittedStake(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	owner := common.HexToAddress("abcd")
	stakingAddress := common.HexToAddress("ffff")
	bzzTokenAddress := common.HexToAddress("eeee")
	nonce := common.BytesToHash(make([]byte, 32))

	expectedCallData, err := stakingContractABI.Pack("stakes", owner)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		prevStake := big.NewInt(100000000000000000)

		contract := staking.New(
			owner,
			stakingAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingAddress {
						if !bytes.Equal(expectedCallData[:64], request.Data[:64]) {
							return nil, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return getPotentialStakeResponse(t, prevStake), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		stakedAmount, err := contract.GetPotentialStake(ctx)
		if err != nil {
			t.Fatal(err)
		}

		if stakedAmount.Cmp(prevStake) != 0 {
			t.Fatalf("expected %v got %v", prevStake, stakedAmount)
		}
	})

	t.Run("error with unpacking", func(t *testing.T) {
		t.Parallel()
		expectedCallData, err := stakingContractABI.Pack("stakes", owner)
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			owner,
			stakingAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingAddress {
						if !bytes.Equal(expectedCallData[:64], request.Data[:64]) {
							return nil, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return []byte{}, nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err = contract.GetPotentialStake(ctx)
		if err == nil {
			t.Fatal("expected error with unpacking")
		}
	})

	t.Run("with invalid call data", func(t *testing.T) {
		t.Parallel()

		addr := swarm.MustParseHexAddress("f30c0aa7e9e2a0ef4c9b1b750ebfeaeb7c7c24da700bb089da19a46e3677824b")

		prevStake := big.NewInt(0)
		expectedCallData, err := stakingContractABI.Pack("stakes", common.BytesToHash(addr.Bytes()))
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			owner,
			stakingAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingAddress {
						if !bytes.Equal(expectedCallData[:64], request.Data[:64]) {
							return nil, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return prevStake.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err = contract.GetPotentialStake(ctx)
		if err == nil {
			t.Fatal("expected error due to wrong call data")
		}
	})

	t.Run("transaction error", func(t *testing.T) {
		t.Parallel()

		contract := staking.New(
			owner,
			stakingAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					return nil, errors.New("some error")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err := contract.GetPotentialStake(ctx)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestGetWithdrawableStake(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	owner := common.HexToAddress("abcd")
	stakingAddress := common.HexToAddress("ffff")
	bzzTokenAddress := common.HexToAddress("eeee")
	nonce := common.BytesToHash(make([]byte, 32))

	expectedCallData, err := stakingContractABI.Pack("withdrawableStake")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		prevStake := big.NewInt(100000000000000000)

		contract := staking.New(
			owner,
			stakingAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingAddress {
						if !bytes.Equal(expectedCallData[:32], request.Data[:32]) {
							return nil, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return prevStake.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		withdrawableStake, err := contract.GetWithdrawableStake(ctx)
		if err != nil {
			t.Fatal(err)
		}

		if withdrawableStake.Cmp(prevStake) != 0 {
			t.Fatalf("expected %v got %v", prevStake, withdrawableStake)
		}
	})

	t.Run("error with unpacking", func(t *testing.T) {
		t.Parallel()
		expectedCallData, err := stakingContractABI.Pack("withdrawableStake")
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			owner,
			stakingAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingAddress {
						if !bytes.Equal(expectedCallData[:32], request.Data[:32]) {
							return nil, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return []byte{}, nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err = contract.GetPotentialStake(ctx)
		if err == nil {
			t.Fatal("expected error with unpacking")
		}
	})

	t.Run("transaction error", func(t *testing.T) {
		t.Parallel()

		contract := staking.New(
			owner,
			stakingAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					return nil, errors.New("some error")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err := contract.GetPotentialStake(ctx)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestWithdrawStake(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	owner := common.HexToAddress("abcd")
	stakingContractAddress := common.HexToAddress("ffff")
	bzzTokenAddress := common.HexToAddress("eeee")
	nonce := common.BytesToHash(make([]byte, 32))
	withdrawableStake := big.NewInt(100000000000000000)

	t.Run("ok", func(t *testing.T) {
		t.Parallel()
		txHashWithdrawn := common.HexToHash("c3a1")

		expectedCallDataForWithdraw, err := stakingContractABI.Pack("withdrawFromStake")
		if err != nil {
			t.Fatal(err)
		}
		expectedCallDataForGetStake, err := stakingContractABI.Pack("withdrawableStake")
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == stakingContractAddress {
						if !bytes.Equal(expectedCallDataForWithdraw[:], request.Data[:]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallDataForWithdraw, request.Data)
						}
						return txHashWithdrawn, nil
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashWithdrawn {
						return &types.Receipt{
							Status: 1,
						}, nil
					}
					return nil, errors.New("unknown tx hash")
				}),
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingContractAddress {
						if bytes.Equal(expectedCallDataForGetStake[:32], request.Data[:32]) {
							return withdrawableStake.FillBytes(make([]byte, 32)), nil
						}
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err = contract.WithdrawStake(ctx)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("has no stake", func(t *testing.T) {
		t.Parallel()

		invalidStakedAmount := big.NewInt(0)

		expectedCallDataForGetStake, err := stakingContractABI.Pack("withdrawableStake")
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingContractAddress {
						if bytes.Equal(expectedCallDataForGetStake[:32], request.Data[:32]) {
							return invalidStakedAmount.FillBytes(make([]byte, 32)), nil
						}
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err = contract.WithdrawStake(ctx)
		if !errors.Is(err, staking.ErrInsufficientStake) {
			t.Fatal(err)
		}
	})

	t.Run("send tx failed", func(t *testing.T) {
		t.Parallel()
		txHashWithdrawn := common.HexToHash("c3a1")

		expectedCallDataForWithdraw, err := stakingContractABI.Pack("withdrawFromStake")
		if err != nil {
			t.Fatal(err)
		}
		expectedCallDataForGetStake, err := stakingContractABI.Pack("withdrawableStake")
		if err != nil {
			t.Fatal(err)
		}

		expectedErr := errors.New("tx err")

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == stakingContractAddress {
						if !bytes.Equal(expectedCallDataForWithdraw[:], request.Data[:]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallDataForWithdraw, request.Data)
						}
						return common.Hash{}, fmt.Errorf("send tx failed: %w", expectedErr)
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashWithdrawn {
						return &types.Receipt{
							Status: 1,
						}, nil
					}
					return nil, errors.New("unknown tx hash")
				}),
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingContractAddress {
						if bytes.Equal(expectedCallDataForGetStake[:32], request.Data[:32]) {
							return withdrawableStake.FillBytes(make([]byte, 32)), nil
						}
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err = contract.WithdrawStake(ctx)
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected err %v, got %v", expectedErr, err)
		}
	})

	t.Run("tx reverted", func(t *testing.T) {
		t.Parallel()
		txHashWithdrawn := common.HexToHash("c3a1")

		expectedCallDataForWithdraw, err := stakingContractABI.Pack("withdrawFromStake")
		if err != nil {
			t.Fatal(err)
		}
		expectedCallDataForGetStake, err := stakingContractABI.Pack("withdrawableStake")
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == stakingContractAddress {
						if !bytes.Equal(expectedCallDataForWithdraw[:], request.Data[:]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallDataForWithdraw, request.Data)
						}
						return txHashWithdrawn, nil
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashWithdrawn {
						return &types.Receipt{
							Status: 0,
						}, nil
					}
					return nil, errors.New("unknown tx hash")
				}),
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingContractAddress {
						if bytes.Equal(expectedCallDataForGetStake[:32], request.Data[:32]) {
							return withdrawableStake.FillBytes(make([]byte, 32)), nil
						}
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err = contract.WithdrawStake(ctx)
		if err == nil {
			t.Fatalf("expected non nil error, got nil")
		}
	})

	t.Run("get stake with err", func(t *testing.T) {
		t.Parallel()

		expectedCallDataForGetStake, err := stakingContractABI.Pack("withdrawableStake")
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingContractAddress {
						if bytes.Equal(expectedCallDataForGetStake[:32], request.Data[:32]) {
							return nil, fmt.Errorf("some error")
						}
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err = contract.WithdrawStake(ctx)
		if err == nil {
			t.Fatalf("expected non nil error, got nil")
		}
	})
}

func TestMigrateStake(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	owner := common.HexToAddress("abcd")
	stakingContractAddress := common.HexToAddress("ffff")
	bzzTokenAddress := common.HexToAddress("eeee")
	nonce := common.BytesToHash(make([]byte, 32))
	stakedAmount := big.NewInt(100000000000000000)

	t.Run("ok", func(t *testing.T) {

		expectedCallDataForPaused, err := stakingContractABI.Pack("paused")
		if err != nil {
			t.Fatal(err)
		}
		expectedCallDataForWithdraw, err := stakingContractABI.Pack("migrateStake")
		if err != nil {
			t.Fatal(err)
		}
		expectedCallDataForGetStake, err := stakingContractABI.Pack("nodeEffectiveStake", owner)
		if err != nil {
			t.Fatal(err)
		}

		t.Parallel()
		txHashWithdrawn := common.HexToHash("c3a1")
		expected := big.NewInt(1)

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == stakingContractAddress {
						if !bytes.Equal(expectedCallDataForWithdraw[:], request.Data[:]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallDataForWithdraw, request.Data)
						}
						return txHashWithdrawn, nil
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashWithdrawn {
						return &types.Receipt{
							Status: 1,
						}, nil
					}
					return nil, errors.New("unknown tx hash")
				}),
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingContractAddress {
						if bytes.Equal(expectedCallDataForPaused[:], request.Data[:]) {
							return expected.FillBytes(make([]byte, 32)), nil
						}
						if bytes.Equal(expectedCallDataForGetStake[:64], request.Data[:64]) {
							return stakedAmount.FillBytes(make([]byte, 32)), nil
						}
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err = contract.MigrateStake(ctx)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("is paused", func(t *testing.T) {
		t.Parallel()
		expected := big.NewInt(0)

		expectedCallDataForPaused, err := stakingContractABI.Pack("paused")
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingContractAddress {
						if bytes.Equal(expectedCallDataForPaused[:], request.Data[:]) {
							return expected.FillBytes(make([]byte, 32)), nil
						}
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err = contract.MigrateStake(ctx)
		if !errors.Is(err, staking.ErrNotPaused) {
			t.Fatal(err)
		}
	})

	t.Run("invalid call data", func(t *testing.T) {
		t.Parallel()
		_, err := stakingContractABI.Pack("paused", owner)
		if err == nil {
			t.Fatalf("expected non nil error, got nil")
		}
		_, err = stakingContractABI.Pack("migrateStake", owner)
		if err == nil {
			t.Fatalf("expected non nil error, got nil")
		}

		_, err = stakingContractABI.Pack("nodeEffectiveStake", stakedAmount)
		if err == nil {
			t.Fatalf("expected non nil error, got nil")
		}
	})

	t.Run("send tx failed", func(t *testing.T) {
		t.Parallel()
		txHashWithdrawn := common.HexToHash("c3a1")
		expected := big.NewInt(1)

		expectedCallDataForPaused, err := stakingContractABI.Pack("paused")
		if err != nil {
			t.Fatal(err)
		}
		expectedCallDataForWithdraw, err := stakingContractABI.Pack("migrateStake")
		if err != nil {
			t.Fatal(err)
		}
		expectedCallDataForGetStake, err := stakingContractABI.Pack("nodeEffectiveStake", owner)
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == stakingContractAddress {
						if !bytes.Equal(expectedCallDataForWithdraw[:], request.Data[:]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallDataForWithdraw, request.Data)
						}
						return common.Hash{}, errors.New("send tx failed")
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashWithdrawn {
						return &types.Receipt{
							Status: 1,
						}, nil
					}
					return nil, errors.New("unknown tx hash")
				}),
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingContractAddress {
						if bytes.Equal(expectedCallDataForPaused[:], request.Data[:]) {
							return expected.FillBytes(make([]byte, 32)), nil
						}
						if bytes.Equal(expectedCallDataForGetStake[:64], request.Data[:64]) {
							return stakedAmount.FillBytes(make([]byte, 32)), nil
						}
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err = contract.MigrateStake(ctx)
		if err == nil {
			t.Fatalf("expected non nil error, got nil")
		}
	})

	t.Run("tx reverted", func(t *testing.T) {

		expectedCallDataForPaused, err := stakingContractABI.Pack("paused")
		if err != nil {
			t.Fatal(err)
		}
		expectedCallDataForWithdraw, err := stakingContractABI.Pack("migrateStake")
		if err != nil {
			t.Fatal(err)
		}
		expectedCallDataForGetStake, err := stakingContractABI.Pack("nodeEffectiveStake", owner)
		if err != nil {
			t.Fatal(err)
		}

		t.Parallel()
		txHashWithdrawn := common.HexToHash("c3a1")
		expected := big.NewInt(1)

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == stakingContractAddress {
						if !bytes.Equal(expectedCallDataForWithdraw[:], request.Data[:]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallDataForWithdraw, request.Data)
						}
						return txHashWithdrawn, nil
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashWithdrawn {
						return &types.Receipt{
							Status: 0,
						}, nil
					}
					return nil, errors.New("unknown tx hash")
				}),
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingContractAddress {
						if bytes.Equal(expectedCallDataForPaused[:], request.Data[:]) {
							return expected.FillBytes(make([]byte, 32)), nil
						}
						if bytes.Equal(expectedCallDataForGetStake[:64], request.Data[:64]) {
							return stakedAmount.FillBytes(make([]byte, 32)), nil
						}
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err = contract.MigrateStake(ctx)
		if err == nil {
			t.Fatalf("expected non nil error, got nil")
		}
	})

	t.Run("is paused with err", func(t *testing.T) {

		expectedCallDataForPaused, err := stakingContractABI.Pack("paused")
		if err != nil {
			t.Fatal(err)
		}

		t.Parallel()

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingContractAddress {
						if bytes.Equal(expectedCallDataForPaused[:], request.Data[:]) {
							return nil, fmt.Errorf("some error")
						}
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err = contract.WithdrawStake(ctx)
		if err == nil {
			t.Fatalf("expected non nil error, got nil")
		}
	})

	t.Run("get stake with err", func(t *testing.T) {

		expectedCallDataForPaused, err := stakingContractABI.Pack("paused")
		if err != nil {
			t.Fatal(err)
		}
		expectedCallDataForGetStake, err := stakingContractABI.Pack("nodeEffectiveStake", owner)
		if err != nil {
			t.Fatal(err)
		}

		t.Parallel()
		expected := big.NewInt(1)

		contract := staking.New(
			owner,
			stakingContractAddress,
			stakingContractABI,
			bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingContractAddress {
						if bytes.Equal(expectedCallDataForPaused[:], request.Data[:]) {
							return expected.FillBytes(make([]byte, 32)), nil
						}
						if bytes.Equal(expectedCallDataForGetStake[:64], request.Data[:64]) {
							return nil, fmt.Errorf("some error")
						}
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
			false,
			stakingHeight,
		)

		_, err = contract.MigrateStake(ctx)
		if err == nil {
			t.Fatalf("expected non nil error, got nil")
		}
	})
}

func getPotentialStakeResponse(t *testing.T, amount *big.Int) []byte {
	t.Helper()

	ret := make([]byte, 32+32+32+32+32+32)
	copy(ret, swarm.RandAddress(t).Bytes())
	copy(ret[64:], amount.FillBytes(make([]byte, 32)))

	return ret
}
