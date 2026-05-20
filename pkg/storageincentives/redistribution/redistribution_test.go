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
	"github.com/stretchr/testify/assert"
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
				transactionMock.WithSendWithRetryFunc(func(ctx context.Context, request *transaction.TxRequest) (common.Hash, *types.Receipt, error) {
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
				transactionMock.WithSendWithRetryFunc(func(ctx context.Context, request *transaction.TxRequest) (common.Hash, *types.Receipt, error) {
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
				transactionMock.WithSendWithRetryFunc(func(ctx context.Context, request *transaction.TxRequest) (common.Hash, *types.Receipt, error) {
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
				transactionMock.WithSendWithRetryFunc(func(ctx context.Context, request *transaction.TxRequest) (common.Hash, *types.Receipt, error) {
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
				transactionMock.WithSendWithRetryFunc(func(ctx context.Context, request *transaction.TxRequest) (common.Hash, *types.Receipt, error) {
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
			0,
		)

		_, err = contract.Commit(ctx, actualHash, 0)
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
		100_000,
		time.Millisecond,
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

const testShortBlockTime = 10 * time.Millisecond

// 1. Commit waits while cost too high; after EstimateTxCost improves, tx is sent successfully.
func TestCommit_RetriesUntilCostAcceptableThenSuccess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	owner := common.HexToAddress("abcd")
	overlay := swarm.NewAddress(common.HexToHash("cbd").Bytes())
	redistributionContractAddress := common.HexToAddress("ffff")
	testobfus := common.Hex2Bytes("hash")
	var obfus [32]byte
	copy(obfus[:], testobfus)
	expectedHash := common.HexToHash("abc123")

	var estimateCalls atomic.Int32
	txSvc := transactionMock.New(
		transactionMock.WithEstimateTxCostFunc(func(_ context.Context, gasUnits int64, _ int) (*big.Int, *big.Int, error) {
			n := estimateCalls.Add(1)
			var gasFeeCap *big.Int
			if n == 1 {
				gasFeeCap = big.NewInt(10)
			} else {
				gasFeeCap = big.NewInt(1)
			}
			cost := new(big.Int).Mul(big.NewInt(gasUnits), gasFeeCap)
			return cost, gasFeeCap, nil
		}),
		transactionMock.WithSendFunc(
			func(ctx context.Context, request *transaction.TxRequest, _ int) (common.Hash, error) {
				assert.NotNil(t, request.GasFeeCap)
				assert.Equal(t, 0, big.NewInt(1).Cmp(request.GasFeeCap), "fee cap must match last successful estimate")

				callData, err := redistributionContractABI.Pack("commit", obfus, uint64(0))
				assert.NoError(t, err)
				assert.Equal(t, callData, request.Data)
				return expectedHash, nil
			}),
		transactionMock.WithWaitForReceiptFunc(func(context.Context, common.Hash) (*types.Receipt, error) {
			return &types.Receipt{Status: 1}, nil
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
		testShortBlockTime,
		redistribution.WithMaxTxCost(500_000, 0),
	)

	h, err := c.Commit(ctx, testobfus, 0)
	assert.NoError(t, err)
	assert.Equal(t, expectedHash, h)
	assert.EqualValues(t, 2, estimateCalls.Load(), "should re-estimate after blockTime wait")
}

// 2. Commit sends a tx but receives a critical error → fails without endless retry.
func TestCommit_CriticalErrorFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	owner := common.HexToAddress("abcd")
	overlay := swarm.NewAddress(common.HexToHash("cbd").Bytes())
	redistributionContractAddress := common.HexToAddress("ffff")
	testobfus := common.Hex2Bytes("hash")

	txSvc := transactionMock.New(
		transactionMock.WithSendFunc(func(context.Context, *transaction.TxRequest, int) (common.Hash, error) {
			return common.Hash{}, transaction.ErrTransactionReverted
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
		testShortBlockTime,
	)

	_, err := c.Commit(ctx, testobfus, 0)
	assert.Error(t, err)
	assert.ErrorIs(t, err, transaction.ErrTransactionReverted)
}

// First Send fails with non-critical error and zero tx hash; wait blockTime, retry Send -> success.
func TestCommit_RetriesAfterTransientSendFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	owner := common.HexToAddress("abcd")
	overlay := swarm.NewAddress(common.HexToHash("cbd").Bytes())
	redistributionContractAddress := common.HexToAddress("ffff")
	testobfus := common.Hex2Bytes("hash")
	expectedHash := common.HexToHash("deadbeef")

	var sendCalls atomic.Int32
	txSvc := transactionMock.New(
		transactionMock.WithEstimateTxCostFunc(func(_ context.Context, gasUnits int64, _ int) (*big.Int, *big.Int, error) {
			gasFeeCap := big.NewInt(2)
			return new(big.Int).Mul(big.NewInt(gasUnits), gasFeeCap), gasFeeCap, nil
		}),
		transactionMock.WithSendFunc(func(context.Context, *transaction.TxRequest, int) (common.Hash, error) {
			if sendCalls.Add(1) == 1 {
				// Zero hash + non-critical error must trigger a second Send after blockTime.
				return common.Hash{}, errors.New("temporary rpc failure")
			}
			return expectedHash, nil
		}),
		transactionMock.WithWaitForReceiptFunc(func(context.Context, common.Hash) (*types.Receipt, error) {
			return &types.Receipt{Status: 1}, nil
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
		testShortBlockTime,
		redistribution.WithMaxTxCost(2_000_000, 0),
	)

	h, err := c.Commit(ctx, testobfus, 0)
	assert.NoError(t, err)
	assert.Equal(t, expectedHash, h)
	assert.EqualValues(t, 2, sendCalls.Load(), "second Send should happen after zero-hash failure")
}

// 4. Commit never sends: cost stays above max-tx-cap until context is cancelled.
func TestCommit_contextCancelledWhileWaitingForAcceptableCost(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	owner := common.HexToAddress("abcd")
	overlay := swarm.NewAddress(common.HexToHash("cbd").Bytes())
	redistributionContractAddress := common.HexToAddress("ffff")

	txSvc := transactionMock.New(
		transactionMock.WithEstimateTxCostFunc(func(_ context.Context, gasUnits int64, _ int) (*big.Int, *big.Int, error) {
			gasFeeCap := big.NewInt(100)
			return new(big.Int).Mul(big.NewInt(gasUnits), gasFeeCap), gasFeeCap, nil
		}),
		transactionMock.WithSendFunc(func(context.Context, *transaction.TxRequest, int) (common.Hash, error) {
			t.Fatal("Send must not be called")
			return common.Hash{}, nil
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
		testShortBlockTime,
		redistribution.WithMaxTxCost(500_000, 0),
	)

	_, err := c.Commit(ctx, common.Hex2Bytes("hash"), 0)
	assert.ErrorIs(t, err, redistribution.ErrMaxTxCostExceeded)
}

// No max-tx-cost limit: user pays whatever fee the network/request carries (nil caps on TxRequest).
func TestCommit_withoutMaxTxCostLeavesFeeCapsUnset(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	owner := common.HexToAddress("abcd")
	overlay := swarm.NewAddress(common.HexToHash("cbd").Bytes())
	redistributionContractAddress := common.HexToAddress("ffff")
	testobfus := common.Hex2Bytes("hash")
	expectedHash := common.HexToHash("bbbb")

	txSvc := transactionMock.New(
		transactionMock.WithSendFunc(func(_ context.Context, request *transaction.TxRequest, _ int) (common.Hash, error) {
			assert.Nil(t, request.GasFeeCap)
			assert.Nil(t, request.GasPrice)
			return expectedHash, nil
		}),
		transactionMock.WithWaitForReceiptFunc(func(context.Context, common.Hash) (*types.Receipt, error) {
			return &types.Receipt{Status: 1}, nil
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
		testShortBlockTime,
	)

	h, err := c.Commit(ctx, testobfus, 0)
	assert.NoError(t, err)
	assert.Equal(t, expectedHash, h)
}

// 6. Claim hits max cost; there is 10 blocks remaining before phase end, expectedReward covers upper-bound cost + previous round fees → limit bypassed, tx sent.
func TestClaim_bypassesMaxTxCost(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	owner := common.HexToAddress("abcd")
	overlay := swarm.NewAddress(common.HexToHash("cbd").Bytes())
	redistributionContractAddress := common.HexToAddress("ffff")
	proofs := randChunkInclusionProofs(t)
	expectedHash := common.HexToHash("cafe")

	var sendCalls atomic.Int32
	txSvc := transactionMock.New(
		transactionMock.WithEstimateTxCostFunc(func(_ context.Context, gasUnits int64, _ int) (*big.Int, *big.Int, error) {
			gasFeeCap := big.NewInt(20)
			return new(big.Int).Mul(big.NewInt(gasUnits), gasFeeCap), gasFeeCap, nil
		}),
		transactionMock.WithSendFunc(func(_ context.Context, request *transaction.TxRequest, _ int) (common.Hash, error) {
			sendCalls.Add(1)
			callData, err := redistributionContractABI.Pack("claim", proofs.A, proofs.B, proofs.C)
			assert.NoError(t, err)
			assert.Equal(t, callData, request.Data)
			assert.NotNil(t, request.GasFeeCap, "override should pin maxFeePerGas for send")
			return expectedHash, nil
		}),
		transactionMock.WithWaitForReceiptFunc(func(context.Context, common.Hash) (*types.Receipt, error) {
			return &types.Receipt{Status: 1}, nil
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
		testShortBlockTime,
		redistribution.WithMaxTxCost(500_000, 0),
	)

	opts := &redistribution.ClaimOpts{
		OverrideAfterBlock: 100,
		CurrentBlockFn:     func() uint64 { return 110 },
		ExpectedReward:     new(big.Int).Mul(big.NewInt(50), big.NewInt(1_000_000)), // >> estimated + fees
		RoundFees:          big.NewInt(100_000),
	}

	h, err := c.Claim(ctx, proofs, opts)
	assert.NoError(t, err)
	assert.Equal(t, expectedHash, h)
	assert.EqualValues(t, 1, sendCalls.Load())
}

// 7. Same high fees; expected reward does not cover cost — override denied; cancelled ctx exits without Send.
func TestClaim_noBypassWhenRewardTooSmall(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	owner := common.HexToAddress("abcd")
	overlay := swarm.NewAddress(common.HexToHash("cbd").Bytes())
	redistributionContractAddress := common.HexToAddress("ffff")
	proofs := randChunkInclusionProofs(t)

	var sendCalls atomic.Int32
	txSvc := transactionMock.New(
		transactionMock.WithEstimateTxCostFunc(func(_ context.Context, gasUnits int64, _ int) (*big.Int, *big.Int, error) {
			gasFeeCap := big.NewInt(50)
			return new(big.Int).Mul(big.NewInt(gasUnits), gasFeeCap), gasFeeCap, nil
		}),
		transactionMock.WithSendFunc(func(context.Context, *transaction.TxRequest, int) (common.Hash, error) {
			sendCalls.Add(1)
			return common.Hash{}, errors.New("Send must not run")
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
		testShortBlockTime,
		redistribution.WithMaxTxCost(500_000, 0),
	)

	opts := &redistribution.ClaimOpts{
		OverrideAfterBlock: 100,
		CurrentBlockFn:     func() uint64 { return 200 },
		ExpectedReward:     big.NewInt(1000),
		RoundFees:          big.NewInt(1),
	}

	_, err := c.Claim(ctx, proofs, opts)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, sendCalls.Load())
}
