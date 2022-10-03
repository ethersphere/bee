// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redistributioncontract

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/transaction"
	transactionMock "github.com/ethersphere/bee/pkg/transaction/mock"
)

func TestRedistribution(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	owner := swarm.MustParseHexAddress("f30c0aa7e9e2a0ef4c9b1b750ebfeaeb7c7c24da700bb089da19a46e3677824b")
	redistributionAddress := common.HexToAddress("ffff")
	//nonce := common.BytesToHash(make([]byte, 32))
	txHashDeposited := common.HexToHash("c3a7")

	t.Run("IsPlaying - true", func(t *testing.T) {
		t.Parallel()

		depth := uint8(10)

		expectedRes := big.NewInt(1)
		contract := New(owner, log.NewLogger(loggerName),
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == redistributionAddress {
						return expectedRes.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				})),
			redistributionAddress)

		isPlaying, err := contract.IsPlaying(ctx, depth)
		if err != nil {
			t.Fatal(err)
		}
		if !isPlaying {
			t.Fatal("expected playing")
		}
	})

	t.Run("IsPlaying - false", func(t *testing.T) {
		t.Parallel()

		depth := uint8(10)

		expectedRes := big.NewInt(0)
		contract := New(owner, log.NewLogger(loggerName),
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == redistributionAddress {
						return expectedRes.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				})),
			redistributionAddress)

		isPlaying, err := contract.IsPlaying(ctx, depth)
		if err != nil {
			t.Fatal(err)
		}
		if isPlaying {
			t.Fatal("expected not playing")
		}
	})

	t.Run("IsWinner - false", func(t *testing.T) {
		t.Parallel()

		expectedRes := big.NewInt(0)
		contract := New(owner, log.NewLogger(loggerName),
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == redistributionAddress {
						return expectedRes.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				})),
			redistributionAddress)

		isWinner, err := contract.IsWinner(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if isWinner {
			t.Fatalf("expected false, got %t", isWinner)
		}
	})
	//Fix this at the end
	t.Run("IsWinner - true", func(t *testing.T) {
		t.Parallel()

		expectedCallData, err := redistributionContractABI.Pack("isWinner", getOverlayAddr(owner))
		if err != nil {
			t.Fatal(err)
		}
		expectedRes := big.NewInt(1)
		contract := New(owner, log.NewLogger(loggerName),
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest) (txHash common.Hash, err error) {
					if *request.To == redistributionAddress {
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
					return nil, errors.New("unknown tx hash")
				}),
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == redistributionAddress {
						return expectedRes.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				})),
			redistributionAddress)

		isWinner, err := contract.IsWinner(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !isWinner {
			t.Fatalf("expected true, got %t", isWinner)
		}
	})

	t.Run("Claim", func(t *testing.T) {
		t.Parallel()

		expectedCallData, err := redistributionContractABI.Pack("claim")
		if err != nil {
			t.Fatal(err)
		}
		contract := New(owner, log.NewLogger(loggerName),
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest) (txHash common.Hash, err error) {
					if *request.To == redistributionAddress {
						if !bytes.Equal(expectedCallData[:32], request.Data[:32]) {
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
					return nil, errors.New("unknown tx hash")
				}),
			),
			redistributionAddress)

		err = contract.Claim(ctx)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Claim with tx reverted", func(t *testing.T) {
		t.Parallel()

		expectedCallData, err := redistributionContractABI.Pack("claim")
		if err != nil {
			t.Fatal(err)
		}
		contract := New(owner, log.NewLogger(loggerName),
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest) (txHash common.Hash, err error) {
					if *request.To == redistributionAddress {
						if !bytes.Equal(expectedCallData[:32], request.Data[:32]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return txHashDeposited, nil
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
				transactionMock.WithWaitForReceiptFunc(func(ctx context.Context, txHash common.Hash) (receipt *types.Receipt, err error) {
					if txHash == txHashDeposited {
						return &types.Receipt{
							Status: 0,
						}, nil
					}
					return nil, errors.New("unknown tx hash")
				}),
			),
			redistributionAddress)

		err = contract.Claim(ctx)
		if err != transaction.ErrTransactionReverted {
			t.Fatal(err)
		}
	})

	t.Run("Commit", func(t *testing.T) {
		t.Parallel()
		var obfus [32]byte
		testobfus := common.Hex2Bytes("hash")
		copy(obfus[:], testobfus)
		expectedCallData, err := redistributionContractABI.Pack("commit", obfus, getOverlayAddr(owner))
		if err != nil {
			t.Fatal(err)
		}
		contract := New(owner, log.NewLogger(loggerName),
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest) (txHash common.Hash, err error) {
					if *request.To == redistributionAddress {
						if !bytes.Equal(expectedCallData[:32], request.Data[:32]) {
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
					return nil, errors.New("unknown tx hash")
				}),
			),
			redistributionAddress)

		err = contract.Commit(ctx, testobfus)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Reveal", func(t *testing.T) {
		t.Parallel()

		reserveCommitmentHash := common.Hex2Bytes("hash")
		var hash [32]byte
		copy(hash[:], reserveCommitmentHash)

		randomNonce := common.Hex2Bytes("nonce")
		var randNonce [32]byte
		copy(randNonce[:], randomNonce)

		depth := uint8(10)

		expectedCallData, err := redistributionContractABI.Pack("reveal", getOverlayAddr(owner), depth, hash, randNonce)
		if err != nil {
			t.Fatal(err)
		}
		contract := New(owner, log.NewLogger(loggerName),
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest) (txHash common.Hash, err error) {
					if *request.To == redistributionAddress {
						if !bytes.Equal(expectedCallData[:32], request.Data[:32]) {
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
					return nil, errors.New("unknown tx hash")
				}),
			),
			redistributionAddress)

		err = contract.Reveal(ctx, depth, reserveCommitmentHash, randomNonce)
		if err != nil {
			t.Fatal(err)
		}
	})
}
