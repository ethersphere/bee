// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wrapped

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/transaction/backendmock"
	"github.com/stretchr/testify/assert"
)

const (
	testBlockTime         = 5 * time.Second
	testBlockSyncInterval = uint64(20)
	testMinimumGasTipCap  = 0
)

func Test_BlockNumberCache_MissLoadsFromRPC(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const expectedBlock = uint64(10)
		var headerCalls atomic.Int32

		backend := newTestWrappedBackend(t, backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
			headerCalls.Add(1)
			assert.Nil(t, number)
			return &types.Header{
				Number: big.NewInt(int64(expectedBlock)),
				Time:   uint64(time.Now().UTC().Unix()),
			}, nil
		}))

		got, err := backend.BlockNumber(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, expectedBlock, got)
		assert.Equal(t, int32(1), headerCalls.Load())
	})
}

func Test_BlockNumberReturns_FreshCache(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const cachedBlock = uint64(20)
		var headerCalls atomic.Int32

		backend := newTestWrappedBackend(t, backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
			headerCalls.Add(1)
			return nil, errors.New("unexpected rpc call")
		}))

		now := time.Now().UTC()
		setBlockAnchor(backend, blockNumberAnchor{
			number:    cachedBlock,
			timestamp: now,
		})

		got, err := backend.BlockNumber(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, cachedBlock, got)
		assert.Zero(t, headerCalls.Load())
	})
}

func Test_BlockNumber_ReturnsCalculatedBlock(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const (
			anchorBlock   = uint64(30)
			elapsedBlocks = uint64(3)
			expectedBlock = anchorBlock + elapsedBlocks
		)
		var headerCalls atomic.Int32

		backend := newTestWrappedBackend(t, backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
			headerCalls.Add(1)
			return nil, errors.New("unexpected rpc call")
		}))

		now := time.Now().UTC()
		setBlockAnchor(backend, blockNumberAnchor{
			number:    anchorBlock,
			timestamp: now.Add(-time.Duration(elapsedBlocks) * testBlockTime),
		})

		got, err := backend.BlockNumber(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, expectedBlock, got)
		assert.Zero(t, headerCalls.Load())
	})
}

func Test_BlockNumber_ExpiredAnchor(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const (
			staleBlock    = uint64(40)
			freshBlock    = uint64(100)
			elapsedBlocks = testBlockSyncInterval + 1
		)
		var headerCalls atomic.Int32

		backend := newTestWrappedBackend(t, backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
			headerCalls.Add(1)
			return &types.Header{
				Number: big.NewInt(int64(freshBlock)),
				Time:   uint64(time.Now().UTC().Unix()),
			}, nil
		}))

		now := time.Now().UTC()
		setBlockAnchor(backend, blockNumberAnchor{
			number:    staleBlock,
			timestamp: now.Add(-time.Duration(elapsedBlocks) * testBlockTime),
		})

		got, err := backend.BlockNumber(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, freshBlock, got)
		assert.Equal(t, int32(1), headerCalls.Load())
	})
}

func Test_BlockNumber_ExpiredAnchor_RetriesAfterRPCError(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const (
			staleBlock     = uint64(50)
			recoveredBlock = uint64(200)
			elapsedBlocks  = testBlockSyncInterval + 1
		)
		rpcErr := errors.New("rpc unavailable")
		var headerCalls atomic.Int32

		backend := newTestWrappedBackend(t, backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
			call := headerCalls.Add(1)
			if call == 1 {
				return nil, rpcErr
			}
			return &types.Header{
				Number: big.NewInt(int64(recoveredBlock)),
				Time:   uint64(time.Now().UTC().Unix()),
			}, nil
		}))

		now := time.Now().UTC()
		setBlockAnchor(backend, blockNumberAnchor{
			number:    staleBlock,
			timestamp: now.Add(-time.Duration(elapsedBlocks) * testBlockTime),
		})

		first, err := backend.BlockNumber(context.Background())

		assert.ErrorIs(t, err, rpcErr)
		assert.Zero(t, first)
		assert.Equal(t, int32(1), headerCalls.Load())

		second, err := backend.BlockNumber(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, recoveredBlock, second)
		assert.Equal(t, int32(2), headerCalls.Load())
	})
}

func Test_BlockNumber_UsesAverageBlockTime(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const genesisBlock = uint64(100)

		const (
			realBlockTime     = 7 * time.Second
			configBlockTime   = 2 * time.Second
			blockSyncInterval = uint64(3)
			cacheHitAdvance   = 4 * time.Second
			untilSecondPoll   = 4 * time.Second
		)

		// Chain produces blocks every 7s; goroutine must exit before the bubble ends.
		chain := newChainSimulator(genesisBlock, realBlockTime)
		go chain.advanceEvery(realBlockTime, 1)
		synctest.Wait()

		var headerCalls atomic.Int32
		backend := newTestWrappedBackendWithConfig(
			t,
			configBlockTime,
			blockSyncInterval,
			backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
				headerCalls.Add(1)
				assert.Nil(t, number)
				return chain.header(), nil
			}),
		)

		// Step 1: initial poll, anchor at block 100, uses pre-set blocktime for further calculations
		first, err := backend.BlockNumber(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, genesisBlock, first)
		assert.Equal(t, int32(1), headerCalls.Load())

		time.Sleep(cacheHitAdvance)
		synctest.Wait()

		// Step 2: cache hit with config block time (2s) — node estimates 102, chain is at 100.
		ahead, err := backend.BlockNumber(context.Background())
		assert.NoError(t, err)
		assert.Greater(t, ahead, chain.blockNumber())

		time.Sleep(untilSecondPoll)
		synctest.Wait()

		// Step 3: cache expired, second poll — average block time becomes 7s, estimate realigns to 101.
		corrected, err := backend.BlockNumber(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, chain.blockNumber(), corrected)
		assert.Equal(t, int32(2), headerCalls.Load())

		time.Sleep(cacheHitAdvance)
		synctest.Wait()

		// Step 4: cache hit uses average block time: block from shouldn't be ahead
		steady, err := backend.BlockNumber(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, chain.blockNumber(), steady)
		assert.NotEqual(t, corrected+1, steady)
		assert.Equal(t, int32(2), headerCalls.Load())
	})
}

func Test_BlockNumber_AverageBlockTimeWhenChainHasNotAdvanced(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const (
			genesisBlock      = uint64(100)
			configBlockTime   = 2 * time.Second
			blockSyncInterval = uint64(3)
			untilSecondPoll   = 6 * time.Second
		)

		chain := newChainSimulator(genesisBlock, 7*time.Second)

		var headerCalls atomic.Int32
		backend := newTestWrappedBackendWithConfig(
			t,
			configBlockTime,
			blockSyncInterval,
			backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
				headerCalls.Add(1)
				return chain.header(), nil
			}),
		)

		first, err := backend.BlockNumber(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, genesisBlock, first)

		time.Sleep(untilSecondPoll)
		synctest.Wait()

		second, err := backend.BlockNumber(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, genesisBlock, second)
		assert.Equal(t, int32(2), headerCalls.Load())

		time.Sleep(4 * time.Second)
		synctest.Wait()

		// With observed ~6s average, extrapolation stays at 100. With config 2s it would reach 102.
		third, err := backend.BlockNumber(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, genesisBlock, third)
		assert.NotEqual(t, genesisBlock+2, third)
	})
}

type chainSimulator struct {
	mu           sync.Mutex
	genesisTime  time.Time
	genesisBlock uint64
	block        uint64
	blockTime    time.Duration
}

func newChainSimulator(genesisBlock uint64, blockTime time.Duration) *chainSimulator {
	return &chainSimulator{
		genesisTime:  time.Now().UTC(),
		genesisBlock: genesisBlock,
		block:        genesisBlock,
		blockTime:    blockTime,
	}
}

func (c *chainSimulator) advanceEvery(blockTime time.Duration, count int) {
	for range count {
		time.Sleep(blockTime)
		c.mu.Lock()
		c.block++
		c.mu.Unlock()
	}
}

func (c *chainSimulator) header() *types.Header {
	c.mu.Lock()
	defer c.mu.Unlock()

	blockOffset := c.block - c.genesisBlock
	return &types.Header{
		Number: big.NewInt(int64(c.block)),
		Time:   uint64(c.genesisTime.Add(c.blockTime * time.Duration(blockOffset)).Unix()),
	}
}

func (c *chainSimulator) blockNumber() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.block
}

func setBlockAnchor(backend *wrappedBackend, anchor blockNumberAnchor) {
	backend.blockNumberCache.Set(anchor)
}

func newTestWrappedBackend(t *testing.T, opts ...backendmock.Option) *wrappedBackend {
	t.Helper()

	return newTestWrappedBackendWithConfig(t, testBlockTime, testBlockSyncInterval, opts...)
}

func newTestWrappedBackendWithConfig(
	t *testing.T,
	blockTime time.Duration,
	blockSyncInterval uint64,
	opts ...backendmock.Option,
) *wrappedBackend {
	t.Helper()

	backend, ok := NewBackend(
		backendmock.New(opts...),
		testMinimumGasTipCap,
		blockTime,
		blockSyncInterval,
	).(*wrappedBackend)
	assert.True(t, ok)

	return backend
}
