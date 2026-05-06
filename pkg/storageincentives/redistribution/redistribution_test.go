// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redistribution_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	chaincfg "github.com/ethersphere/bee/v2/pkg/config"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/sctx"
	"github.com/ethersphere/bee/v2/pkg/storageincentives/redistribution"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	transactionMock "github.com/ethersphere/bee/v2/pkg/transaction/mock"
	"github.com/ethersphere/bee/v2/pkg/util/abiutil"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

var (
	redistributionContractABI = abiutil.MustParseABI(chaincfg.Testnet.RedistributionABI)
	postageContractABI        = abiutil.MustParseABI(chaincfg.Testnet.PostageStampABI)
	postageContractAddress    = common.HexToAddress("eeee")
)

func randChunkInclusionProof(t *testing.T) redistribution.ChunkInclusionProof {
	t.Helper()

	return redistribution.ChunkInclusionProof{
		ProofSegments:  []common.Hash{common.BytesToHash(testutil.RandBytes(t, 32))},
		ProveSegment:   common.BytesToHash(testutil.RandBytes(t, 32)),
		ProofSegments2: []common.Hash{common.BytesToHash(testutil.RandBytes(t, 32))},
		ProveSegment2:  common.BytesToHash(testutil.RandBytes(t, 32)),
		ProofSegments3: []common.Hash{common.BytesToHash(testutil.RandBytes(t, 32))},
		PostageProof: redistribution.PostageProof{
			Signature: testutil.RandBytes(t, 65),
			PostageId: common.BytesToHash(testutil.RandBytes(t, 32)),
			Index:     binary.BigEndian.Uint64(testutil.RandBytes(t, 8)),
			TimeStamp: binary.BigEndian.Uint64(testutil.RandBytes(t, 8)),
		},
		ChunkSpan: 1,
		SocProof:  []redistribution.SOCProof{},
	}
}

func randChunkInclusionProofs(t *testing.T) redistribution.ChunkInclusionProofs {
	t.Helper()

	return redistribution.ChunkInclusionProofs{
		A: randChunkInclusionProof(t),
		B: randChunkInclusionProof(t),
		C: randChunkInclusionProof(t),
	}
}

func TestRedistribution(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = sctx.SetGasPrice(ctx, big.NewInt(100))
	owner := common.HexToAddress("abcd")
	overlay := swarm.NewAddress(common.HexToHash("cbd").Bytes())
	redistributionContractAddress := common.HexToAddress("ffff")
	// nonce := common.BytesToHash(make([]byte, 32))
	txHashDeposited := common.HexToHash("c3a7")

	t.Run("IsPlaying - true", func(t *testing.T) {
		t.Parallel()

		depth := uint8(10)
		expectedRes := big.NewInt(1)
		contract := redistribution.New(
			overlay,
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
			postageContractAddress,
			postageContractABI,
			0,
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
			overlay,
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
			postageContractAddress,
			postageContractABI,
			0,
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
			overlay,
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
			postageContractAddress,
			postageContractABI,
			0,
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
		contract := redistribution.New(overlay, owner, log.Noop,
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
			postageContractAddress,
			postageContractABI,
			0,
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

		proofs := randChunkInclusionProofs(t)

		expectedCallData, err := redistributionContractABI.Pack("claim", proofs.A, proofs.B, proofs.C)
		if err != nil {
			t.Fatal(err)
		}
		contract := redistribution.New(
			overlay,
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
			postageContractAddress,
			postageContractABI,
			0,
		)

		_, err = contract.Claim(ctx, proofs, nil)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Claim with tx reverted", func(t *testing.T) {
		t.Parallel()

		proofs := randChunkInclusionProofs(t)
		expectedCallData, err := redistributionContractABI.Pack("claim", proofs.A, proofs.B, proofs.C)
		if err != nil {
			t.Fatal(err)
		}
		contract := redistribution.New(
			overlay,
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
			postageContractAddress,
			postageContractABI,
			0,
		)

		_, err = contract.Claim(ctx, proofs, nil)
		if !errors.Is(err, transaction.ErrTransactionReverted) {
			t.Fatal(err)
		}
	})

	t.Run("Commit", func(t *testing.T) {
		t.Parallel()
		var obfus [32]byte
		testobfus := common.Hex2Bytes("hash")
		copy(obfus[:], testobfus)
		expectedCallData, err := redistributionContractABI.Pack("commit", obfus, uint64(0))
		if err != nil {
			t.Fatal(err)
		}
		contract := redistribution.New(
			overlay,
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
			postageContractAddress,
			postageContractABI,
			0,
		)

		_, err = contract.Commit(ctx, testobfus, 0)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Reveal", func(t *testing.T) {
		t.Parallel()

		reserveCommitmentHash := common.BytesToHash(common.Hex2Bytes("hash"))
		randomNonce := common.BytesToHash(common.Hex2Bytes("nonce"))
		depth := uint8(10)

		expectedCallData, err := redistributionContractABI.Pack("reveal", depth, reserveCommitmentHash, randomNonce)
		if err != nil {
			t.Fatal(err)
		}
		contract := redistribution.New(
			overlay,
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
			postageContractAddress,
			postageContractABI,
			0,
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
			overlay,
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
			postageContractAddress,
			postageContractABI,
			0,
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
			overlay,
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
			postageContractAddress,
			postageContractABI,
			0,
		)

		_, err := contract.IsPlaying(ctx, depth)
		if err == nil {
			t.Fatal("expecting error")
		}
	})

	t.Run("invalid call data", func(t *testing.T) {
		t.Parallel()

		expectedCallData, err := redistributionContractABI.Pack("commit", common.BytesToHash(common.Hex2Bytes("some hash")), uint64(0))
		if err != nil {
			t.Fatal(err)
		}
		contract := redistribution.New(
			overlay,
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
			postageContractAddress,
			postageContractABI,
			0,
		)

		_, err = contract.Commit(ctx, common.Hex2Bytes("hash"), 0)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestRedistribution_MaxTxCostWaitsUntilContextDone(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	owner := common.HexToAddress("abcd")
	overlay := swarm.NewAddress(common.HexToHash("cbd").Bytes())
	redistributionContractAddress := common.HexToAddress("ffff")
	var sendCalled atomic.Bool

	proofs := randChunkInclusionProofs(t)

	contract := redistribution.New(
		overlay,
		owner,
		log.Noop,
		transactionMock.New(
			transactionMock.WithEstimateTxCostFunc(func(_ context.Context, gasUnits int64, _ int) (*big.Int, *big.Int, error) {
				gasFeeCap := big.NewInt(10)
				return new(big.Int).Mul(big.NewInt(gasUnits), gasFeeCap), gasFeeCap, nil
			}),
			transactionMock.WithSendFunc(func(context.Context, *transaction.TxRequest, int) (common.Hash, error) {
				sendCalled.Store(true)
				return common.Hash{}, errors.New("send should not be called")
			}),
			transactionMock.WithWaitForReceiptFunc(func(context.Context, common.Hash) (*types.Receipt, error) {
				return nil, errors.New("unexpected wait")
			}),
		),
		redistributionContractAddress,
		redistributionContractABI,
		postageContractAddress,
		postageContractABI,
		100_000,
		redistribution.WithMaxTxCost(500_000, 0),
	)

	_, err := contract.Claim(ctx, proofs, nil)
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("want context deadline or cancel, got %v", err)
	}
	if sendCalled.Load() {
		t.Fatal("Send must not be called when cost exceeds limit")
	}
}
