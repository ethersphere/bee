// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redistribution_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	chaincfg "github.com/ethersphere/bee/pkg/config"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/storageincentives/redistribution"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/transaction"
	transactionMock "github.com/ethersphere/bee/pkg/transaction/mock"
	"github.com/ethersphere/bee/pkg/util/abiutil"
	"github.com/ethersphere/bee/pkg/util/testutil"
)

var redistributionContractABI = abiutil.MustParseABI(chaincfg.Testnet.RedistributionABI)

func TestRedistribution(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = sctx.SetGasPrice(ctx, big.NewInt(100))
	owner := swarm.MustParseHexAddress("f30c0aa7e9e2a0ef4c9b1b750ebfeaeb7c7c24da700bb089da19a46e3677824b")
	redistributionContractAddress := common.HexToAddress("ffff")
	//nonce := common.BytesToHash(make([]byte, 32))
	txHashDeposited := common.HexToHash("c3a7")

	t.Run("IsPlaying - true", func(t *testing.T) {
		t.Parallel()

		depth := uint8(10)
		expectedRes := big.NewInt(1)
		contract := redistribution.New(
			owner,
			log.Noop,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == redistributionContractAddress {
						return expectedRes.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			redistributionContractAddress,
			redistributionContractABI,
		)

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
		contract := redistribution.New(
			owner,
			log.Noop,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == redistributionContractAddress {
						return expectedRes.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			redistributionContractAddress,
			redistributionContractABI,
		)

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
		contract := redistribution.New(
			owner,
			log.Noop,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == redistributionContractAddress {
						return expectedRes.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			redistributionContractAddress,
			redistributionContractABI,
		)

		isWinner, err := contract.IsWinner(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if isWinner {
			t.Fatalf("expected false, got %t", isWinner)
		}
	})

	t.Run("IsWinner - true", func(t *testing.T) {
		t.Parallel()

		expectedRes := big.NewInt(1)
		contract := redistribution.New(owner, log.Noop,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == redistributionContractAddress {
						return expectedRes.FillBytes(make([]byte, 32)), nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			redistributionContractAddress,
			redistributionContractABI,
		)

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

		proofs := redistribution.RandChunkInclusionProofs(t)
		// TODO_PH4: use this when abi is updated
		// expectedCallData, err := redistributionContractABI.Pack("claim", proofs.A, proofs.B, proofs.C)
		expectedCallData, err := redistributionContractABI.Pack("claim")
		if err != nil {
			t.Fatal(err)
		}
		contract := redistribution.New(
			owner,
			log.Noop,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == redistributionContractAddress {
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
			redistributionContractAddress,
			redistributionContractABI,
		)

		_, err = contract.Claim(ctx, proofs)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Claim with tx reverted", func(t *testing.T) {
		t.Parallel()

		proofs := redistribution.RandChunkInclusionProofs(t)
		// TODO_PH4: use this when abi is updated
		// expectedCallData, err := redistributionContractABI.Pack("claim", proofs.A, proofs.B, proofs.C)
		expectedCallData, err := redistributionContractABI.Pack("claim")
		if err != nil {
			t.Fatal(err)
		}
		contract := redistribution.New(
			owner,
			log.Noop,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == redistributionContractAddress {
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
			redistributionContractAddress,
			redistributionContractABI,
		)

		_, err = contract.Claim(ctx, proofs)
		if !errors.Is(err, transaction.ErrTransactionReverted) {
			t.Fatal(err)
		}
	})

	t.Run("Commit", func(t *testing.T) {
		t.Parallel()
		var obfus [32]byte
		testobfus := common.Hex2Bytes("hash")
		copy(obfus[:], testobfus)
		expectedCallData, err := redistributionContractABI.Pack("commit", obfus, common.BytesToHash(owner.Bytes()), big.NewInt(0))
		if err != nil {
			t.Fatal(err)
		}
		contract := redistribution.New(
			owner,
			log.Noop,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == redistributionContractAddress {
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
			redistributionContractAddress,
			redistributionContractABI,
		)

		_, err = contract.Commit(ctx, testobfus, big.NewInt(0))
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Reveal", func(t *testing.T) {
		t.Parallel()

		reserveCommitmentHash := common.BytesToHash(common.Hex2Bytes("hash"))
		randomNonce := common.BytesToHash(common.Hex2Bytes("nonce"))
		depth := uint8(10)

		expectedCallData, err := redistributionContractABI.Pack("reveal", common.BytesToHash(owner.Bytes()), depth, reserveCommitmentHash, randomNonce)
		if err != nil {
			t.Fatal(err)
		}
		contract := redistribution.New(
			owner,
			log.Noop,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, _ int) (txHash common.Hash, err error) {
					if *request.To == redistributionContractAddress {
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
			redistributionContractAddress,
			redistributionContractABI,
		)

		_, err = contract.Reveal(ctx, depth, common.Hex2Bytes("hash"), common.Hex2Bytes("nonce"))
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Reserve Salt", func(t *testing.T) {
		t.Parallel()
		someSalt := testutil.RandBytes(t, 32)
		contract := redistribution.New(
			owner,
			log.Noop,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == redistributionContractAddress {

						return someSalt, nil
					}
					return nil, errors.New("unexpected call")
				}),
			),
			redistributionContractAddress,
			redistributionContractABI,
		)

		salt, err := contract.ReserveSalt(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(salt, someSalt) {
			t.Fatal("expected bytes to be equal")
		}
	})

	t.Run("send tx failed", func(t *testing.T) {
		t.Parallel()

		depth := uint8(10)
		contract := redistribution.New(
			owner,
			log.Noop,
			transactionMock.New(
				transactionMock.WithCallFunc(func(ctx context.Context, request *transaction.TxRequest) (result []byte, err error) {
					if *request.To == redistributionContractAddress {
						return nil, errors.New("some error")
					}
					return nil, errors.New("unexpected call")
				}),
			),
			redistributionContractAddress,
			redistributionContractABI,
		)

		_, err := contract.IsPlaying(ctx, depth)
		if err == nil {
			t.Fatal("expecting error")
		}
	})

	t.Run("invalid call data", func(t *testing.T) {
		t.Parallel()

		expectedCallData, err := redistributionContractABI.Pack("commit", common.BytesToHash(common.Hex2Bytes("some hash")), common.BytesToHash(common.Hex2Bytes("some address")), big.NewInt(0))
		if err != nil {
			t.Fatal(err)
		}
		contract := redistribution.New(
			owner,
			log.Noop,
			transactionMock.New(
				transactionMock.WithSendFunc(func(ctx context.Context, request *transaction.TxRequest, boost int) (txHash common.Hash, err error) {
					if *request.To == redistributionContractAddress {
						if !bytes.Equal(expectedCallData[:], request.Data[:]) {
							return common.Hash{}, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return txHashDeposited, nil
					}
					return common.Hash{}, errors.New("sent to wrong contract")
				}),
			),
			redistributionContractAddress,
			redistributionContractABI,
		)

		_, err = contract.Commit(ctx, common.Hex2Bytes("hash"), big.NewInt(0))
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
