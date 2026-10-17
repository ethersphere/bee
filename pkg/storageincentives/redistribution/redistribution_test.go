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

// sentRequest is what a contract method handed to SendWithRetry, with its retry options applied.
type sentRequest struct {
	request   *transaction.TxRequest
	overrides transaction.RetryOverrides
}

func newRecordingContract(t *testing.T, gasLimit uint64) (redistribution.Contract, *[]sentRequest) {
	t.Helper()

	var sent []sentRequest
	txSvc := transactionMock.New(
		transactionMock.WithSendWithRetryFunc(func(_ context.Context, request *transaction.TxRequest, opts ...transaction.RetryOption) (common.Hash, *types.Receipt, error) {
			var overrides transaction.RetryOverrides
			for _, opt := range opts {
				opt(&overrides)
			}
			sent = append(sent, sentRequest{request: request, overrides: overrides})
			return common.HexToHash("cafe"), &types.Receipt{Status: types.ReceiptStatusSuccessful}, nil
		}),
	)
	c := redistribution.New(
		swarm.NewAddress(common.HexToHash("cbd").Bytes()),
		common.HexToAddress("abcd"),
		log.Noop,
		txSvc,
		common.HexToAddress("ffff"),
		redistributionContractABI,
		gasLimit,
	)
	return c, &sent
}

// gameTransactions sends each redistribution game transaction once.
func gameTransactions(t *testing.T) map[string]func(context.Context, redistribution.Contract) error {
	t.Helper()

	proofs := randChunkInclusionProofs(t)
	return map[string]func(context.Context, redistribution.Contract) error{
		"commit": func(ctx context.Context, c redistribution.Contract) error {
			_, err := c.Commit(ctx, common.Hex2Bytes("hash"), 0)
			return err
		},
		"reveal": func(ctx context.Context, c redistribution.Contract) error {
			_, err := c.Reveal(ctx, 1, common.Hex2Bytes("hash"), common.Hex2Bytes("nonce"))
			return err
		},
		"claim": func(ctx context.Context, c redistribution.Contract) error {
			_, err := c.Claim(ctx, proofs, nil)
			return err
		},
	}
}

// Game transactions have to land inside their phase, so the node retry delay is capped for them.
func TestGameTransactions_RetryDelayCap(t *testing.T) {
	t.Parallel()

	for name, send := range gameTransactions(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, sent := newRecordingContract(t, 0)
			if err := send(context.Background(), c); err != nil {
				t.Fatal(err)
			}
			if len(*sent) != 1 {
				t.Fatalf("expected 1 send, got %d", len(*sent))
			}
			rewrite := (*sent)[0].overrides.RetryDelay
			if rewrite == nil {
				t.Fatal("retry delay is not rewritten")
			}
			if got := rewrite(time.Minute); got != 35*time.Second {
				t.Fatalf("node delay 1m: got %s, want 35s", got)
			}
			if got := rewrite(10 * time.Second); got != 10*time.Second {
				t.Fatalf("node delay 10s: got %s, want 10s", got)
			}
		})
	}
}

// Fees of game transactions come from the retry loop, and the gas limit comes from the
// contract config. Gas settings carried by the context must not leak into the request.
func TestGameTransactions_IgnoreContextGasSettings(t *testing.T) {
	t.Parallel()

	const gasLimit = 400_000
	ctx := sctx.SetGasLimit(sctx.SetGasPrice(context.Background(), big.NewInt(1000)), 9_000_000)

	for name, send := range gameTransactions(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, sent := newRecordingContract(t, gasLimit)
			if err := send(ctx, c); err != nil {
				t.Fatal(err)
			}
			request := (*sent)[0].request
			if request.GasPrice != nil {
				t.Fatalf("gas price from the context must be ignored, got %s", request.GasPrice)
			}
			if request.GasLimit != gasLimit {
				t.Fatalf("got gas limit %d, want the contract gas limit %d", request.GasLimit, gasLimit)
			}
		})
	}
}

// Claim lets the retry loop exceed MaxTxPrice only near the round end, and only when the
// expected reward covers the claim cost plus the fees already spent in the round.
func TestClaim_IgnoreMaxPrice(t *testing.T) {
	t.Parallel()

	gasFeeCap := big.NewInt(1_000_000_000) // 1 gwei
	costAt := func(gasUnits int64) *big.Int {
		return new(big.Int).Mul(gasFeeCap, big.NewInt(gasUnits))
	}
	// opts builds options past the threshold block with the given reward and fees.
	opts := func(reward, fees *big.Int) *redistribution.ClaimOpts {
		return &redistribution.ClaimOpts{
			OverrideAfterBlock: 100,
			CurrentBlockFn:     func() uint64 { return 100 },
			ExpectedReward:     reward,
			RoundFees:          fees,
		}
	}

	tests := []struct {
		name     string
		gasLimit uint64
		opts     *redistribution.ClaimOpts
		want     bool
	}{
		{
			name:     "nil opts",
			gasLimit: 400_000,
			opts:     nil,
			want:     false,
		},
		{
			name:     "zero OverrideAfterBlock disables the override",
			gasLimit: 400_000,
			opts: &redistribution.ClaimOpts{
				CurrentBlockFn: func() uint64 { return 100 },
				ExpectedReward: costAt(800_000),
				RoundFees:      big.NewInt(0),
			},
			want: false,
		},
		{
			name:     "nil CurrentBlockFn disables the override",
			gasLimit: 400_000,
			opts: &redistribution.ClaimOpts{
				OverrideAfterBlock: 100,
				ExpectedReward:     costAt(800_000),
				RoundFees:          big.NewInt(0),
			},
			want: false,
		},
		{
			name:     "nil RoundFees disables the override",
			gasLimit: 400_000,
			opts:     opts(costAt(800_000), nil),
			want:     false,
		},
		{
			name:     "before the threshold block",
			gasLimit: 400_000,
			opts: &redistribution.ClaimOpts{
				OverrideAfterBlock: 100,
				CurrentBlockFn:     func() uint64 { return 99 },
				ExpectedReward:     costAt(800_000),
				RoundFees:          big.NewInt(0),
			},
			want: false,
		},
		{
			name:     "nil reward",
			gasLimit: 400_000,
			opts:     opts(nil, big.NewInt(0)),
			want:     false,
		},
		{
			name:     "zero reward",
			gasLimit: 400_000,
			opts:     opts(big.NewInt(0), big.NewInt(0)),
			want:     false,
		},
		{
			name:     "reward below cost plus round fees",
			gasLimit: 400_000,
			opts:     opts(costAt(400_000), big.NewInt(1)),
			want:     false,
		},
		{
			name:     "reward exactly covers cost plus round fees",
			gasLimit: 400_000,
			opts:     opts(new(big.Int).Add(costAt(400_000), big.NewInt(10)), big.NewInt(10)),
			want:     true,
		},
		{
			name:     "zero gas limit prices the claim at the minimum gas limit",
			gasLimit: 0,
			opts:     opts(costAt(250_000), big.NewInt(0)),
			want:     true,
		},
		{
			name:     "configured gas limit above the minimum raises the cost",
			gasLimit: 500_000,
			opts:     opts(costAt(250_000), big.NewInt(0)),
			want:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c, sent := newRecordingContract(t, tc.gasLimit)
			if _, err := c.Claim(context.Background(), randChunkInclusionProofs(t), tc.opts); err != nil {
				t.Fatal(err)
			}
			ignore := (*sent)[0].overrides.IgnoreMaxPrice
			if ignore == nil {
				t.Fatal("claim must install the max price predicate")
			}
			if got := ignore(gasFeeCap); got != tc.want {
				t.Fatalf("IgnoreMaxPrice(%s) = %v, want %v", gasFeeCap, got, tc.want)
			}
		})
	}
}

// Only claim may exceed MaxTxPrice; commit and reveal keep the node limit.
func TestCommitReveal_KeepMaxPrice(t *testing.T) {
	t.Parallel()

	for name, send := range gameTransactions(t) {
		if name == "claim" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, sent := newRecordingContract(t, 0)
			if err := send(context.Background(), c); err != nil {
				t.Fatal(err)
			}
			if (*sent)[0].overrides.IgnoreMaxPrice != nil {
				t.Fatalf("%s must not bypass MaxTxPrice", name)
			}
		})
	}
}
