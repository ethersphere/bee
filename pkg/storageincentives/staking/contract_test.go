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
	staking2 "github.com/ethersphere/bee/pkg/storageincentives/staking"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/transaction"
	transactionMock "github.com/ethersphere/bee/pkg/transaction/mock"
)

func TestDepositStake(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	owner := common.HexToAddress("abcd")
	stakingAddress := common.HexToAddress("ffff")
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
		expectedCallData, err := staking2.StakingABI.Pack("depositStake", common.BytesToHash(owner.Bytes()), nonce, stakedAmount)
		if err != nil {
			t.Fatal(err)
		}

		contract := staking2.New(addr, owner, stakingAddress, bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == bzzTokenAddress {
						return txHashApprove, nil
					}
					if *request.To == stakingAddress {
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
					if *request.To == stakingAddress {
						return prevStake.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				})),
			nonce)

		_, err = contract.DepositStake(ctx, stakedAmount)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("ok with addon stake", func(t *testing.T) {
		t.Parallel()

		totalAmount := big.NewInt(100000000000000000)
		prevStake := big.NewInt(2)
		expectedCallData, err := staking2.StakingABI.Pack("depositStake", common.BytesToHash(owner.Bytes()), nonce, big.NewInt(100000000000000000))
		if err != nil {
			t.Fatal(err)
		}

		contract := staking2.New(addr, owner, stakingAddress, bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == bzzTokenAddress {
						return txHashApprove, nil
					}
					if *request.To == stakingAddress {
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
					if *request.To == stakingAddress {
						return prevStake.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				})),
			nonce)

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

		contract := staking2.New(addr, owner, stakingAddress, bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == bzzTokenAddress {
						return totalAmount.FillBytes(make([]byte, 32)), nil
					}
					if *request.To == stakingAddress {
						return prevStake.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				})),
			nonce)

		_, err := contract.DepositStake(ctx, big.NewInt(0))
		if !errors.Is(err, staking2.ErrInsufficientStakeAmount) {
			t.Fatal(fmt.Errorf("wanted %w, got %v", staking2.ErrInsufficientStakeAmount, err))
		}
	})

	t.Run("insufficient funds", func(t *testing.T) {
		t.Parallel()

		totalAmount := big.NewInt(0)
		prevStake := big.NewInt(0)

		contract := staking2.New(addr, owner, stakingAddress, bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == bzzTokenAddress {
						return totalAmount.FillBytes(make([]byte, 32)), nil
					}
					if *request.To == stakingAddress {
						return prevStake.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				})),
			nonce)

		_, err := contract.DepositStake(ctx, big.NewInt(100000000000000000))
		if !errors.Is(err, staking2.ErrInsufficientFunds) {
			t.Fatal(fmt.Errorf("wanted %w, got %v", staking2.ErrInsufficientFunds, err))
		}
	})

	t.Run("insufficient stake amount", func(t *testing.T) {
		t.Parallel()

		totalAmount := big.NewInt(0)
		prevStake := big.NewInt(0)

		contract := staking2.New(addr, owner, stakingAddress, bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == bzzTokenAddress {
						return totalAmount.FillBytes(make([]byte, 32)), nil
					}
					if *request.To == stakingAddress {
						return prevStake.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				})),
			nonce)

		_, err := contract.DepositStake(ctx, big.NewInt(100000000000000000))
		if !errors.Is(err, staking2.ErrInsufficientFunds) {
			t.Fatal(fmt.Errorf("wanted %w, got %v", staking2.ErrInsufficientStakeAmount, err))
		}
	})

	t.Run("send tx failed", func(t *testing.T) {
		t.Parallel()

		totalAmount := big.NewInt(102400)
		prevStake := big.NewInt(0)

		contract := staking2.New(addr, owner, stakingAddress, bzzTokenAddress,
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
					if *request.To == stakingAddress {
						return prevStake.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				})),
			nonce)

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
		expectedCallData, err := staking2.Erc20ABI.Pack("approve", stakingAddress, expectedStakeAmount)
		if err != nil {
			t.Fatal(err)
		}

		contract := staking2.New(addr, owner, stakingAddress, bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == bzzTokenAddress {
						if !bytes.Equal(expectedCallData[:], request.Data[:]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return txHashApprove, nil
					}
					if *request.To == stakingAddress {
						return txHashDeposited, nil
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == bzzTokenAddress {
						return totalAmount.FillBytes(make([]byte, 32)), nil
					}
					if *request.To == stakingAddress {
						return prevStake.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				})),
			nonce)

		_, err = contract.DepositStake(ctx, stakedAmount)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("transaction reverted", func(t *testing.T) {
		t.Parallel()

		totalAmount := big.NewInt(100000000000000000)
		prevStake := big.NewInt(0)
		expectedCallData, err := staking2.Erc20ABI.Pack("approve", stakingAddress, stakedAmount)
		if err != nil {
			t.Fatal(err)
		}

		contract := staking2.New(addr, owner, stakingAddress, bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == bzzTokenAddress {
						return txHashApprove, nil
					}
					if *request.To == stakingAddress {
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
							Status: 0,
						}, nil
					}
					return nil, errors.New("unknown tx hash")
				}),
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == bzzTokenAddress {
						return totalAmount.FillBytes(make([]byte, 32)), nil
					}
					if *request.To == stakingAddress {
						return prevStake.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				})),
			nonce)

		_, err = contract.DepositStake(ctx, stakedAmount)
		if !errors.Is(err, transaction.ErrTransactionReverted) {
			t.Fatalf("expeted %v, got %v", transaction.ErrTransactionReverted, err)
		}
	})

	t.Run("transaction error", func(t *testing.T) {
		t.Parallel()

		totalAmount := big.NewInt(102400)
		prevStake := big.NewInt(0)
		expectedCallData, err := staking2.Erc20ABI.Pack("approve", stakingAddress, stakedAmount)
		if err != nil {
			t.Fatal(err)
		}

		contract := staking2.New(addr, owner, stakingAddress, bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == bzzTokenAddress {
						return txHashApprove, nil
					}
					if *request.To == stakingAddress {
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
					if *request.To == stakingAddress {
						return prevStake.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				})),
			nonce)

		_, err = contract.DepositStake(ctx, stakedAmount)
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("transaction error in call", func(t *testing.T) {
		t.Parallel()

		contract := staking2.New(addr, owner, stakingAddress, bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingAddress {
						return nil, errors.New("some error")
					}
					return nil, errors.New("unexpected call")
				})),
			nonce)

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
		expectedCallData, err := staking2.StakingABI.Pack("stakeOfOverlay", common.BytesToHash(addr.Bytes()))
		if err != nil {
			t.Fatal(err)
		}

		contract := staking2.New(addr, owner, stakingAddress, bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingAddress {
						if !bytes.Equal(expectedCallData[:64], request.Data[:64]) {
							return nil, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return prevStake.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				})),
			nonce)

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
		expectedCallData, err := staking2.StakingABI.Pack("stakeOfOverlay", common.BytesToHash(addr.Bytes()))
		if err != nil {
			t.Fatal(err)
		}

		contract := staking2.New(addr, owner, stakingAddress, bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingAddress {
						if !bytes.Equal(expectedCallData[:64], request.Data[:64]) {
							return nil, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return []byte{}, nil
					}
					return nil, errors.New("unexpected call")
				})),
			nonce)

		_, err = contract.GetStake(ctx)
		if err == nil {
			t.Fatal("expected error with unpacking")
		}
	})

	t.Run("with invalid call data", func(t *testing.T) {
		t.Parallel()

		prevStake := big.NewInt(0)
		expectedCallData, err := staking2.StakingABI.Pack("stakeOfOverlay", common.BytesToHash(owner.Bytes()))
		if err != nil {
			t.Fatal(err)
		}

		contract := staking2.New(addr, owner, stakingAddress, bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == stakingAddress {
						if !bytes.Equal(expectedCallData[:64], request.Data[:64]) {
							return nil, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return prevStake.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				})),
			nonce)

		_, err = contract.GetStake(ctx)
		if err == nil {
			t.Fatal("expected error due to wrong call data")
		}
	})

	t.Run("transaction error", func(t *testing.T) {
		t.Parallel()

		contract := staking2.New(addr, owner, stakingAddress, bzzTokenAddress,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					return nil, errors.New("some error")
				})),
			nonce)

		_, err := contract.GetStake(ctx)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
