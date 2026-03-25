// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wrapped

import (
	"context"
	"math/big"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/transaction/backendmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testBlockTime        = 5 * time.Second
	testTTLPercent       = 90
	testMinimumGasTipCap = 0
)

func TestBlockNumberUsesLatestHeaderCache(t *testing.T) {
	t.Parallel()

	const expectedBlock = uint64(10)
	var headerCalls atomic.Int32

	backend := NewBackend(
		backendmock.New(
			backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
				headerCalls.Add(1)
				assert.Nil(t, number)
				return &types.Header{
					Number:  big.NewInt(int64(expectedBlock)),
					Time:    uint64(time.Now().Add(time.Second).Unix()),
					BaseFee: big.NewInt(1),
				}, nil
			}),
		),
		testMinimumGasTipCap,
		testBlockTime,
		testTTLPercent,
	)

	first, err := backend.BlockNumber(context.Background())
	require.NoError(t, err)
	assert.Equal(t, expectedBlock, first)

	second, err := backend.BlockNumber(context.Background())
	require.NoError(t, err)
	assert.Equal(t, expectedBlock, second)
	assert.Equal(t, int32(1), headerCalls.Load())
}

func TestBlockNumberNearExpiry(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		const (
			staleBlock = uint64(100)
			freshBlock = uint64(101)
		)
		var headerCalls atomic.Int32

		now := time.Now()
		backend := NewBackend(
			backendmock.New(
				backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
					headerCalls.Add(1)
					if headerCalls.Load() == 1 {
						return &types.Header{
							Number: big.NewInt(int64(staleBlock)),
							Time:   uint64(now.Add(-testBlockTime).Unix()),
						}, nil
					}
					return &types.Header{
						Number: big.NewInt(int64(freshBlock)),
						Time:   uint64(now.Unix()),
					}, nil
				}),
			),
			testMinimumGasTipCap,
			testBlockTime,
			testTTLPercent,
		)

		ctx := context.Background()
		first, err := backend.BlockNumber(ctx)
		require.NoError(t, err)
		assert.Equal(t, staleBlock, first)

		time.Sleep(testBlockTime/10 + 100*time.Millisecond)

		second, err := backend.BlockNumber(ctx)
		require.NoError(t, err)
		assert.Equal(t, freshBlock, second)
		assert.Equal(t, int32(2), headerCalls.Load())
	})
}

func TestBlockNumberLocalClockBehind(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const expectedBlock = uint64(200)
		var headerCalls atomic.Int32

		backend := NewBackend(
			backendmock.New(
				backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
					headerCalls.Add(1)
					return &types.Header{
						Number: big.NewInt(int64(expectedBlock)),
						Time:   uint64(time.Now().Add(30 * time.Second).Unix()),
					}, nil
				}),
			),
			testMinimumGasTipCap,
			testBlockTime,
			testTTLPercent,
		)

		ttl := testBlockTime * testTTLPercent / 100

		val, err := backend.BlockNumber(context.Background())
		require.NoError(t, err)
		assert.Equal(t, expectedBlock, val)

		time.Sleep(ttl + 100*time.Millisecond)

		_, err = backend.BlockNumber(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int32(2), headerCalls.Load())
	})
}

func TestBlockNumberLocalClockAhead(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {

		const expectedBlock = uint64(300)
		var headerCalls atomic.Int32

		backend := NewBackend(
			backendmock.New(
				backendmock.WithHeaderbyNumberFunc(func(ctx context.Context, number *big.Int) (*types.Header, error) {
					headerCalls.Add(1)
					return &types.Header{
						Number: big.NewInt(int64(expectedBlock)),
						Time:   uint64(time.Now().Add(-2 * testBlockTime).Unix()),
					}, nil
				}),
			),
			testMinimumGasTipCap,
			testBlockTime,
			testTTLPercent,
		)

		val, err := backend.BlockNumber(context.Background())
		require.NoError(t, err)
		assert.Equal(t, expectedBlock, val)

		retryTTL := testBlockTime / 10
		time.Sleep(retryTTL + 100*time.Millisecond)

		_, err = backend.BlockNumber(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int32(2), headerCalls.Load())
	})
}
