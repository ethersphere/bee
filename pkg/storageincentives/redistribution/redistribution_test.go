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
				transactionMock.WithSendWithRetryFunc(func(ctx context.Context, request *transaction.TxRequest, _ ...transaction.RetryOption) (common.Hash, *types.Receipt, error) {
					if *request.To == redistributionContractAddress {
						if !bytes.Equal(expectedCallData[:32], request.Data[:32]) {
							return common.Hash{}, nil, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return txHashDeposited, &types.Receipt{Status: 1}, nil
					}
					return common.Hash{}, nil, errors.New("sent to wrong contract")
				}),
			),
			redistributionContractAddress,
			redistributionContractABI,
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
				transactionMock.WithSendWithRetryFunc(func(ctx context.Context, request *transaction.TxRequest, _ ...transaction.RetryOption) (common.Hash, *types.Receipt, error) {
					if *request.To == redistributionContractAddress {
						if !bytes.Equal(expectedCallData[:32], request.Data[:32]) {
							return common.Hash{}, nil, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return txHashDeposited, &types.Receipt{Status: 0}, transaction.ErrTransactionReverted
					}
					return common.Hash{}, nil, errors.New("sent to wrong contract")
				}),
			),
			redistributionContractAddress,
			redistributionContractABI,
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
				transactionMock.WithSendWithRetryFunc(func(ctx context.Context, request *transaction.TxRequest, _ ...transaction.RetryOption) (common.Hash, *types.Receipt, error) {
					if *request.To == redistributionContractAddress {
						if !bytes.Equal(expectedCallData[:32], request.Data[:32]) {
							return common.Hash{}, nil, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return txHashDeposited, &types.Receipt{Status: 1}, nil
					}
					return common.Hash{}, nil, errors.New("sent to wrong contract")
				}),
			),
			redistributionContractAddress,
			redistributionContractABI,
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
				transactionMock.WithSendWithRetryFunc(func(ctx context.Context, request *transaction.TxRequest, _ ...transaction.RetryOption) (common.Hash, *types.Receipt, error) {
					if *request.To == redistributionContractAddress {
						if !bytes.Equal(expectedCallData[:32], request.Data[:32]) {
							return common.Hash{}, nil, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return txHashDeposited, &types.Receipt{Status: 1}, nil
					}
					return common.Hash{}, nil, errors.New("sent to wrong contract")
				}),
			),
			redistributionContractAddress,
			redistributionContractABI,
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
			0,
		)

		_, err := contract.IsPlaying(ctx, depth)
		if err == nil {
			t.Fatal("expecting error")
		}
	})

	t.Run("invalid call data", func(t *testing.T) {
		t.Parallel()

		// Use valid distinct hashes: Hex2Bytes("some hash") and Hex2Bytes("hash") both decode to empty bytes.
		expectedHash := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		actualHash := common.Hex2Bytes("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
		expectedCallData, err := redistributionContractABI.Pack("commit", expectedHash, uint64(0))
		if err != nil {
			t.Fatal(err)
		}
		contract := redistribution.New(
			overlay,
			owner,
			log.Noop,
			transactionMock.New(
				transactionMock.WithSendWithRetryFunc(func(ctx context.Context, request *transaction.TxRequest, _ ...transaction.RetryOption) (common.Hash, *types.Receipt, error) {
					if *request.To == redistributionContractAddress {
						if !bytes.Equal(expectedCallData, request.Data) {
							return common.Hash{}, nil, fmt.Errorf("got wrong call data. wanted %x, got %x", expectedCallData, request.Data)
						}
						return txHashDeposited, &types.Receipt{Status: 1}, nil
					}
					return common.Hash{}, nil, errors.New("sent to wrong contract")
				}),
			),
			redistributionContractAddress,
			redistributionContractABI,
			0,
		)

		_, err = contract.Commit(ctx, actualHash, 0)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestCommit_CriticalErrorFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	owner := common.HexToAddress("abcd")
	overlay := swarm.NewAddress(common.HexToHash("cbd").Bytes())
	redistributionContractAddress := common.HexToAddress("ffff")
	testobfus := common.Hex2Bytes("hash")

	txSvc := transactionMock.New(
		transactionMock.WithSendWithRetryFunc(func(_ context.Context, _ *transaction.TxRequest, _ ...transaction.RetryOption) (common.Hash, *types.Receipt, error) {
			return common.Hash{}, nil, transaction.ErrTransactionReverted
		}),
	)

	c := redistribution.New(
		overlay,
		owner,
		log.Noop,
		txSvc,
		redistributionContractAddress,
		redistributionContractABI,
		0,
	)

	_, err := c.Commit(ctx, testobfus, 0)
	if !errors.Is(err, transaction.ErrTransactionReverted) {
		t.Fatalf("expected ErrTransactionReverted, got %v", err)
	}
}

func TestCommit_withoutGasFeeCapOnRequest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	owner := common.HexToAddress("abcd")
	overlay := swarm.NewAddress(common.HexToHash("cbd").Bytes())
	redistributionContractAddress := common.HexToAddress("ffff")
	testobfus := common.Hex2Bytes("hash")
	expectedHash := common.HexToHash("bbbb")

	txSvc := transactionMock.New(
		transactionMock.WithSendWithRetryFunc(func(_ context.Context, request *transaction.TxRequest, _ ...transaction.RetryOption) (common.Hash, *types.Receipt, error) {
			if request.GasFeeCap != nil {
				t.Errorf("expected nil GasFeeCap, got %v", request.GasFeeCap)
			}
			if request.GasPrice != nil {
				t.Errorf("expected nil GasPrice, got %v", request.GasPrice)
			}
			return expectedHash, &types.Receipt{Status: 1}, nil
		}),
	)

	c := redistribution.New(
		overlay,
		owner,
		log.Noop,
		txSvc,
		redistributionContractAddress,
		redistributionContractABI,
		0,
	)

	h, err := c.Commit(ctx, testobfus, 0)
	if err != nil {
		t.Fatal(err)
	}
	if h != expectedHash {
		t.Fatalf("got hash %v, want %v", h, expectedHash)
	}
}

func TestClaim_sendsWithRetryOptions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	owner := common.HexToAddress("abcd")
	overlay := swarm.NewAddress(common.HexToHash("cbd").Bytes())
	redistributionContractAddress := common.HexToAddress("ffff")
	proofs := randChunkInclusionProofs(t)
	expectedHash := common.HexToHash("cafe")

	var sendCalls atomic.Int32
	var retryOptsLen int
	txSvc := transactionMock.New(
		transactionMock.WithSendWithRetryFunc(func(_ context.Context, request *transaction.TxRequest, opts ...transaction.RetryOption) (common.Hash, *types.Receipt, error) {
			sendCalls.Add(1)
			retryOptsLen = len(opts)
			callData, err := redistributionContractABI.Pack("claim", proofs.A, proofs.B, proofs.C)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(callData, request.Data) {
				t.Fatalf("got wrong call data. wanted %x, got %x", callData, request.Data)
			}
			return expectedHash, &types.Receipt{Status: 1}, nil
		}),
	)

	c := redistribution.New(
		overlay,
		owner,
		log.Noop,
		txSvc,
		redistributionContractAddress,
		redistributionContractABI,
		0,
	)

	opts := &redistribution.ClaimOpts{
		OverrideAfterBlock: 100,
		CurrentBlockFn:     func() uint64 { return 110 },
		ExpectedReward:     new(big.Int).Mul(big.NewInt(50), big.NewInt(1_000_000)),
		RoundFees:          big.NewInt(100_000),
	}

	h, err := c.Claim(ctx, proofs, opts)
	if err != nil {
		t.Fatal(err)
	}
	if h != expectedHash {
		t.Fatalf("got hash %v, want %v", h, expectedHash)
	}
	if got := sendCalls.Load(); got != 1 {
		t.Fatalf("expected 1 send call, got %d", got)
	}
	if retryOptsLen != 2 {
		t.Fatalf("Claim must pass WithIgnoreMaxPrice and WithRetryDelay retry options, got %d", retryOptsLen)
	}
}

func TestClaim_contextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	owner := common.HexToAddress("abcd")
	overlay := swarm.NewAddress(common.HexToHash("cbd").Bytes())
	redistributionContractAddress := common.HexToAddress("ffff")
	proofs := randChunkInclusionProofs(t)

	txSvc := transactionMock.New(
		transactionMock.WithSendWithRetryFunc(func(ctx context.Context, _ *transaction.TxRequest, _ ...transaction.RetryOption) (common.Hash, *types.Receipt, error) {
			return common.Hash{}, nil, ctx.Err()
		}),
	)

	c := redistribution.New(
		overlay,
		owner,
		log.Noop,
		txSvc,
		redistributionContractAddress,
		redistributionContractABI,
		0,
	)

	opts := &redistribution.ClaimOpts{
		OverrideAfterBlock: 100,
		CurrentBlockFn:     func() uint64 { return 200 },
		ExpectedReward:     big.NewInt(1000),
		RoundFees:          big.NewInt(1),
	}

	_, err := c.Claim(ctx, proofs, opts)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestCanOverrideClaim(t *testing.T) {
	t.Parallel()

	const gasLimit = 250_000

	newContract := func() redistribution.Contract {
		return redistribution.New(
			swarm.NewAddress(common.HexToHash("cbd").Bytes()),
			common.HexToAddress("abcd"),
			log.Noop,
			transactionMock.New(),
			common.HexToAddress("ffff"),
			redistributionContractABI,
			gasLimit,
		)
	}

	// gasFeeCap chosen so that txCost = gasFeeCap * gasLimit is a round number.
	gasFeeCap := big.NewInt(1_000_000_000) // 1 gwei
	txCost := new(big.Int).Mul(gasFeeCap, big.NewInt(gasLimit))

	tests := []struct {
		name string
		opts *redistribution.ClaimOpts
		want bool
	}{
		{
			name: "nil opts",
			opts: nil,
			want: false,
		},
		{
			name: "override disabled when OverrideAfterBlock is zero",
			opts: &redistribution.ClaimOpts{
				CurrentBlockFn: func() uint64 { return 100 },
				ExpectedReward: new(big.Int).Mul(txCost, big.NewInt(2)),
				RoundFees:      big.NewInt(0),
			},
			want: false,
		},
		{
			name: "override disabled when CurrentBlockFn is nil",
			opts: &redistribution.ClaimOpts{
				OverrideAfterBlock: 100,
				ExpectedReward:     new(big.Int).Mul(txCost, big.NewInt(2)),
				RoundFees:          big.NewInt(0),
			},
			want: false,
		},
		{
			name: "override disabled when RoundFees is nil",
			opts: &redistribution.ClaimOpts{
				OverrideAfterBlock: 100,
				CurrentBlockFn:     func() uint64 { return 100 },
				ExpectedReward:     new(big.Int).Mul(txCost, big.NewInt(2)),
			},
			want: false,
		},
		{
			name: "override disabled before threshold block",
			opts: &redistribution.ClaimOpts{
				OverrideAfterBlock: 100,
				CurrentBlockFn:     func() uint64 { return 99 },
				ExpectedReward:     new(big.Int).Mul(txCost, big.NewInt(2)),
				RoundFees:          big.NewInt(0),
			},
			want: false,
		},
		{
			name: "override disabled when reward is nil",
			opts: &redistribution.ClaimOpts{
				OverrideAfterBlock: 100,
				CurrentBlockFn:     func() uint64 { return 100 },
				ExpectedReward:     nil,
				RoundFees:          big.NewInt(0),
			},
			want: false,
		},
		{
			name: "override disabled when reward is zero",
			opts: &redistribution.ClaimOpts{
				OverrideAfterBlock: 100,
				CurrentBlockFn:     func() uint64 { return 100 },
				ExpectedReward:     big.NewInt(0),
				RoundFees:          big.NewInt(0),
			},
			want: false,
		},
		{
			name: "override disabled when reward does not cover cost plus fees",
			opts: &redistribution.ClaimOpts{
				OverrideAfterBlock: 100,
				CurrentBlockFn:     func() uint64 { return 100 },
				ExpectedReward:     txCost, // equals cost, but RoundFees pushes total above reward
				RoundFees:          big.NewInt(1),
			},
			want: false,
		},
		{
			name: "override enabled when reward exactly covers cost plus fees",
			opts: &redistribution.ClaimOpts{
				OverrideAfterBlock: 100,
				CurrentBlockFn:     func() uint64 { return 100 },
				ExpectedReward:     new(big.Int).Add(txCost, big.NewInt(10)),
				RoundFees:          big.NewInt(10),
			},
			want: true,
		},
		{
			name: "override enabled when reward comfortably covers cost",
			opts: &redistribution.ClaimOpts{
				OverrideAfterBlock: 100,
				CurrentBlockFn:     func() uint64 { return 150 },
				ExpectedReward:     new(big.Int).Mul(txCost, big.NewInt(5)),
				RoundFees:          big.NewInt(100),
			},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := redistribution.CanOverrideClaim(newContract(), tc.opts, gasFeeCap)
			if got != tc.want {
				t.Fatalf("CanOverrideClaim() = %v, want %v", got, tc.want)
			}
		})
	}
}
