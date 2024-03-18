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

func TestDepositStake(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	owner := common.HexToAddress("abcd")
	stakingContractAddress := common.HexToAddress("ffff")
	bzzTokenAddress := common.HexToAddress("eeee")
	nonce := common.BytesToHash(make([]byte, 32))
	txHashDeposited := common.HexToHash("c3a7")
	stakedAmount := big.NewInt(100000000000000000)
	addr := swarm.MustParseHexAddress("f30c0aa7e9e2a0ef4c9b1b750ebfeaeb7c7c24da700bb089da19a46e3677824b")
	txHashApprove := common.HexToHash("abb0")

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		totalAmount := big.NewInt(100000000000000000)
		prevStake := big.NewInt(0)
		expectedCallData, err := stakingContractABI.Pack("depositStake", common.BytesToHash(owner.Bytes()), nonce, stakedAmount)
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			addr,
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
						if !bytes.Equal(expectedCallData[:100], request.Data[:100]) {
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
						return prevStake.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
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
		expectedCallData, err := stakingContractABI.Pack("depositStake", common.BytesToHash(owner.Bytes()), nonce, big.NewInt(100000000000000000))
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			addr,
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
						if !bytes.Equal(expectedCallData[:100], request.Data[:100]) {
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
						return prevStake.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
		)

		_, err = contract.DepositStake(ctx, stakedAmount)
		if err != nil {
			t.Fatal(err)
		}
		stakedAmount, err := contract.GetStake(ctx)
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
			addr,
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
						return prevStake.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
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
			addr,
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
						return prevStake.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
		)

		_, err := contract.DepositStake(ctx, big.NewInt(100000000000000000))
		if !errors.Is(err, staking.ErrInsufficientFunds) {
			t.Fatal(fmt.Errorf("wanted %w, got %w", staking.ErrInsufficientFunds, err))
		}
	})

	t.Run("insufficient stake amount", func(t *testing.T) {
		t.Parallel()

		totalAmount := big.NewInt(0)
		prevStake := big.NewInt(0)

		contract := staking.New(
			addr,
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
						return prevStake.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
		)

		_, err := contract.DepositStake(ctx, big.NewInt(100000000000000000))
		if !errors.Is(err, staking.ErrInsufficientFunds) {
			t.Fatal(fmt.Errorf("wanted %w, got %w", staking.ErrInsufficientStakeAmount, err))
		}
	})

	t.Run("send tx failed", func(t *testing.T) {
		t.Parallel()

		totalAmount := big.NewInt(102400)
		prevStake := big.NewInt(0)

		contract := staking.New(
			addr,
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
			addr,
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
			addr,
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
						if !bytes.Equal(expectedCallData[:100], request.Data[:100]) {
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
						return prevStake.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
		)

		_, err = contract.DepositStake(ctx, stakedAmount)
		if !errors.Is(err, transaction.ErrTransactionReverted) {
			t.Fatalf("expeted %v, got %v", transaction.ErrTransactionReverted, err)
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
			addr,
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
						if !bytes.Equal(expectedCallData[:100], request.Data[:100]) {
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
						return prevStake.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
		)

		_, err = contract.DepositStake(ctx, stakedAmount)
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("transaction error in call", func(t *testing.T) {
		t.Parallel()

		contract := staking.New(
			addr,
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
		)

		_, err := contract.DepositStake(ctx, stakedAmount)
		if err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestGetStake(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	owner := common.HexToAddress("abcd")
	stakingAddress := common.HexToAddress("ffff")
	bzzTokenAddress := common.HexToAddress("eeee")
	nonce := common.BytesToHash(make([]byte, 32))
	addr := swarm.MustParseHexAddress("f30c0aa7e9e2a0ef4c9b1b750ebfeaeb7c7c24da700bb089da19a46e3677824b")

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		prevStake := big.NewInt(0)
		expectedCallData, err := stakingContractABI.Pack("stakeOfOverlay", common.BytesToHash(addr.Bytes()))
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			addr,
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
		)

		stakedAmount, err := contract.GetStake(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if stakedAmount.Cmp(big.NewInt(100000000000000000)) == 0 {
			t.Fatalf("expected %v got %v", big.NewInt(100000000000000000), stakedAmount)
		}
	})

	t.Run("error with unpacking", func(t *testing.T) {
		t.Parallel()
		expectedCallData, err := stakingContractABI.Pack("stakeOfOverlay", common.BytesToHash(addr.Bytes()))
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			addr,
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
		)

		_, err = contract.GetStake(ctx)
		if err == nil {
			t.Fatal("expected error with unpacking")
		}
	})

	t.Run("with invalid call data", func(t *testing.T) {
		t.Parallel()

		prevStake := big.NewInt(0)
		expectedCallData, err := stakingContractABI.Pack("stakeOfOverlay", common.BytesToHash(owner.Bytes()))
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			addr,
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
		)

		_, err = contract.GetStake(ctx)
		if err == nil {
			t.Fatal("expected error due to wrong call data")
		}
	})

	t.Run("transaction error", func(t *testing.T) {
		t.Parallel()

		contract := staking.New(
			addr,
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
		)

		_, err := contract.GetStake(ctx)
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
	stakedAmount := big.NewInt(100000000000000000)
	addr := swarm.MustParseHexAddress("f30c0aa7e9e2a0ef4c9b1b750ebfeaeb7c7c24da700bb089da19a46e3677824b")
	txHashApprove := common.HexToHash("abb0")

	t.Run("ok", func(t *testing.T) {
		t.Parallel()
		txHashWithdrawn := common.HexToHash("c3a1")
		expected := big.NewInt(1)

		expectedCallDataForPaused, err := stakingContractABI.Pack("paused")
		if err != nil {
			t.Fatal(err)
		}

		expectedCallDataForWithdraw, err := stakingContractABI.Pack("withdrawFromStake", common.BytesToHash(addr.Bytes()), stakedAmount)
		if err != nil {
			t.Fatal(err)
		}

		expectedCallDataForGetStake, err := stakingContractABI.Pack("stakeOfOverlay", common.BytesToHash(addr.Bytes()))
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			addr,
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
						if !bytes.Equal(expectedCallDataForWithdraw[:], request.Data[:]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallDataForWithdraw, request.Data)
						}
						return txHashWithdrawn, nil
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashApprove {
						return &types.Receipt{
							Status: 1,
						}, nil
					}
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
		)

		_, err = contract.WithdrawAllStake(ctx)
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
			addr,
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
		)

		_, err = contract.WithdrawAllStake(ctx)
		if !errors.Is(err, staking.ErrNotPaused) {
			t.Fatal(err)
		}
	})

	t.Run("has no stake", func(t *testing.T) {
		t.Parallel()
		expected := big.NewInt(1)

		expectedCallDataForPaused, err := stakingContractABI.Pack("paused")
		if err != nil {
			t.Fatal(err)
		}

		invalidStakedAmount := big.NewInt(0)

		expectedCallDataForGetStake, err := stakingContractABI.Pack("stakeOfOverlay", common.BytesToHash(addr.Bytes()))
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			addr,
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
							return invalidStakedAmount.FillBytes(make([]byte, 32)), nil
						}
					}
					return nil, errors.New("unexpected call")
				}),
			),
			nonce,
		)

		_, err = contract.WithdrawAllStake(ctx)
		if !errors.Is(err, staking.ErrInsufficientStake) {
			t.Fatal(err)
		}
	})

	t.Run("invalid call data", func(t *testing.T) {
		t.Parallel()
		_, err := stakingContractABI.Pack("paused", addr)
		if err == nil {
			t.Fatal(err)
		}
		_, err = stakingContractABI.Pack("withdrawFromStake", stakedAmount)
		if err == nil {
			t.Fatal(err)
		}

		_, err = stakingContractABI.Pack("stakeOfOverlay", stakedAmount)
		if err == nil {
			t.Fatal(err)
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

		expectedCallDataForWithdraw, err := stakingContractABI.Pack("withdrawFromStake", common.BytesToHash(addr.Bytes()), stakedAmount)
		if err != nil {
			t.Fatal(err)
		}

		expectedCallDataForGetStake, err := stakingContractABI.Pack("stakeOfOverlay", common.BytesToHash(addr.Bytes()))
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			addr,
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
						if !bytes.Equal(expectedCallDataForWithdraw[:], request.Data[:]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallDataForWithdraw, request.Data)
						}
						return common.Hash{}, errors.New("send tx failed")
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashApprove {
						return &types.Receipt{
							Status: 1,
						}, nil
					}
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
		)

		_, err = contract.WithdrawAllStake(ctx)
		if err == nil {
			t.Fatal(err)
		}
	})

	t.Run("tx reverted", func(t *testing.T) {
		t.Parallel()
		txHashWithdrawn := common.HexToHash("c3a1")
		expected := big.NewInt(1)

		expectedCallDataForPaused, err := stakingContractABI.Pack("paused")
		if err != nil {
			t.Fatal(err)
		}

		expectedCallDataForGetStake, err := stakingContractABI.Pack("stakeOfOverlay", common.BytesToHash(addr.Bytes()))
		if err != nil {
			t.Fatal(err)
		}

		expectedCallDataForWithdraw, err := stakingContractABI.Pack("withdrawFromStake", common.BytesToHash(addr.Bytes()), stakedAmount)
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			addr,
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
						if !bytes.Equal(expectedCallDataForWithdraw[:], request.Data[:]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallDataForWithdraw, request.Data)
						}
						return txHashWithdrawn, nil
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashApprove {
						return &types.Receipt{
							Status: 1,
						}, nil
					}
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
		)

		_, err = contract.WithdrawAllStake(ctx)
		if err == nil {
			t.Fatal(err)
		}
	})

	t.Run("is paused with err", func(t *testing.T) {
		t.Parallel()
		expectedCallDataForPaused, err := stakingContractABI.Pack("paused")
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			addr,
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
		)

		_, err = contract.WithdrawAllStake(ctx)
		if err == nil {
			t.Fatal(err)
		}
	})

	t.Run("get stake with err", func(t *testing.T) {
		t.Parallel()
		expected := big.NewInt(1)

		expectedCallDataForPaused, err := stakingContractABI.Pack("paused")
		if err != nil {
			t.Fatal(err)
		}

		expectedCallDataForGetStake, err := stakingContractABI.Pack("stakeOfOverlay", common.BytesToHash(addr.Bytes()))
		if err != nil {
			t.Fatal(err)
		}

		contract := staking.New(
			addr,
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
		)

		_, err = contract.WithdrawAllStake(ctx)
		if err == nil {
			t.Fatal(err)
		}
	})
}
