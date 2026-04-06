// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wrapped

import (
	"context"
	"errors"
	"math/big"
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
		backend.blockNumberCache.Set(blockNumberAnchor{
			number:    cachedBlock,
			timestamp: now,
		}, now.Add(testBlockTime))

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
		backend.blockNumberCache.Set(blockNumberAnchor{
			number:    anchorBlock,
			timestamp: now.Add(-time.Duration(elapsedBlocks) * testBlockTime),
		}, now.Add(-time.Second))

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
		backend.blockNumberCache.Set(blockNumberAnchor{
			number:    staleBlock,
			timestamp: now.Add(-time.Duration(elapsedBlocks) * testBlockTime),
		}, now.Add(-time.Second))

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
		backend.blockNumberCache.Set(blockNumberAnchor{
			number:    staleBlock,
			timestamp: now.Add(-time.Duration(elapsedBlocks) * testBlockTime),
		}, now.Add(-time.Second))

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

func newTestWrappedBackend(t *testing.T, opts ...backendmock.Option) *wrappedBackend {
	t.Helper()

	backend, ok := NewBackend(
		backendmock.New(opts...),
		testMinimumGasTipCap,
		testBlockTime,
		testBlockSyncInterval,
	).(*wrappedBackend)
	assert.True(t, ok)

	return backend
}
