// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testMetricsPrefix = "test"

func newTestCache() *ExpiringSingleFlightCache[uint64] {
	return &ExpiringSingleFlightCache[uint64]{
		key:     testMetricsPrefix,
		metrics: newMetricSet(testMetricsPrefix),
	}
}

func TestPeekOrLoadHit(t *testing.T) {
	t.Parallel()

	now := time.Now()
	c := newTestCache()
	c.Set(42, now)

	var loadCount atomic.Int32
	val, err := c.PeekOrLoad(
		context.Background(),
		func(value uint64, expiresAt time.Time) (bool, time.Time) {
			return true, expiresAt
		},
		func() (uint64, time.Time, error) {
			loadCount.Add(1)
			return 0, time.Time{}, errors.New("loader must not run")
		},
	)

	require.NoError(t, err)
	assert.Equal(t, uint64(42), val)
	assert.Zero(t, loadCount.Load())
}

func TestPeekOrLoadMiss(t *testing.T) {
	t.Parallel()

	c := newTestCache()
	var loadCount atomic.Int32

	val, err := c.PeekOrLoad(
		context.Background(),
		func(value uint64, expiresAt time.Time) (bool, time.Time) {
			return false, time.Time{}
		},
		func() (uint64, time.Time, error) {
			loadCount.Add(1)
			return 99, time.Now().Add(time.Second * 30), nil
		},
	)

	require.NoError(t, err)
	assert.Equal(t, uint64(99), val)
	assert.Equal(t, int32(1), loadCount.Load())

	var verifyLoads atomic.Int32
	got, err := c.PeekOrLoad(
		context.Background(),
		func(value uint64, expiresAt time.Time) (bool, time.Time) {
			now := time.Now()
			return now.Before(expiresAt), expiresAt
		},
		func() (uint64, time.Time, error) {
			verifyLoads.Add(1)
			return 0, time.Time{}, errors.New("unexpected load on verify")
		},
	)
	require.NoError(t, err)
	assert.Equal(t, uint64(99), got)
	assert.Zero(t, verifyLoads.Load())
}

func TestPeekOrLoadError(t *testing.T) {
	t.Parallel()

	c := newTestCache()
	errLoad := errors.New("load failed")

	var loadCount atomic.Int32
	val, err := c.PeekOrLoad(
		context.Background(),
		func(value uint64, expiresAt time.Time) (bool, time.Time) {
			return false, time.Time{}
		},
		func() (uint64, time.Time, error) {
			loadCount.Add(1)
			return 99, time.Now().Add(time.Second * 30), errLoad
		},
	)

	assert.ErrorIs(t, err, errLoad)
	assert.Equal(t, uint64(0), val)
	assert.Equal(t, int32(1), loadCount.Load())

	// check value returned with error wasn't cached
	_, err = c.PeekOrLoad(
		context.Background(),
		func(value uint64, expiresAt time.Time) (bool, time.Time) {
			return false, time.Time{}
		},
		func() (uint64, time.Time, error) {
			loadCount.Add(1)
			return 0, time.Time{}, errLoad
		},
	)
	assert.ErrorIs(t, err, errLoad)
	assert.Equal(t, int32(2), loadCount.Load())
}

func TestPeekOrLoadSingleflight(t *testing.T) {
	const value = uint64(77)
	synctest.Test(t, func(t *testing.T) {
		c := newTestCache()
		var loadCount atomic.Int32
		gate := make(chan struct{})

		const n = 10
		var wg sync.WaitGroup
		results := make([]uint64, n)
		errs := make([]error, n)

		wg.Add(n)
		for i := range n {
			go func(idx int) {
				defer wg.Done()
				now := time.Now()

				results[idx], errs[idx] = c.PeekOrLoad(
					context.Background(),
					func(value uint64, expiresAt time.Time) (bool, time.Time) {
						return now.Before(expiresAt), expiresAt
					},
					func() (uint64, time.Time, error) {
						loadCount.Add(1)
						<-gate
						return value, now.Add(time.Second), nil
					},
				)
			}(i)
		}

		time.Sleep(50 * time.Millisecond)
		close(gate)
		wg.Wait()

		assert.Equal(t, int32(1), loadCount.Load())
		for i := range n {
			assert.NoError(t, errs[i])
			assert.Equal(t, value, results[i])
		}
	})
}

func TestPeekOrLoadContextCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const expectedVal = 55
		c := newTestCache()
		gate := make(chan struct{})

		ctx1, cancel1 := context.WithCancel(context.Background())
		ctx2 := context.Background()

		var wg sync.WaitGroup
		var result1, result2 uint64
		var err1, err2 error

		wg.Add(2)
		go func() {
			defer wg.Done()

			result1, err1 = c.PeekOrLoad(
				ctx1,
				func(value uint64, expiresAt time.Time) (bool, time.Time) {
					now := time.Now()
					return now.Before(expiresAt), expiresAt
				},
				func() (uint64, time.Time, error) {
					<-gate
					return expectedVal, time.Now().Add(time.Second), nil
				},
			)
		}()

		go func() {
			defer wg.Done()
			result2, err2 = c.PeekOrLoad(
				ctx2,
				func(value uint64, expiresAt time.Time) (bool, time.Time) {
					now := time.Now()
					return now.Before(expiresAt), expiresAt
				},
				func() (uint64, time.Time, error) {
					<-gate
					return expectedVal, time.Now().Add(time.Second), nil
				},
			)
		}()

		time.Sleep(50 * time.Millisecond)
		cancel1()
		time.Sleep(50 * time.Millisecond)
		close(gate)
		wg.Wait()

		assert.ErrorIs(t, err1, context.Canceled)
		assert.Equal(t, uint64(0), result1)

		require.NoError(t, err2)
		assert.Equal(t, uint64(expectedVal), result2)
	})
}
