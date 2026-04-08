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

func TestSetPeek(t *testing.T) {
	t.Parallel()

	now := time.Now()
	expiresAt := now.Add(time.Second)
	c := newTestCache()
	c.Set(42, expiresAt)

	val, gotExpiresAt, ok := c.Peek()
	assert.True(t, ok)
	assert.Equal(t, uint64(42), val)
	assert.Equal(t, expiresAt, gotExpiresAt)
}

func TestPeekReturnsExpiredValue(t *testing.T) {
	t.Parallel()

	now := time.Now()
	c := newTestCache()
	c.Set(42, now)

	val, expiresAt, ok := c.Peek()
	assert.True(t, ok)
	assert.Equal(t, uint64(42), val)
	assert.Equal(t, now, expiresAt)
}

func TestPeekOrLoadMiss(t *testing.T) {
	t.Parallel()

	c := newTestCache()
	var loadCount atomic.Int32

	val, err := c.PeekOrLoad(
		context.Background(),
		time.Now(),
		func(value uint64, expiresAt, now time.Time) (bool, time.Time) {
			return false, time.Time{}
		},
		func() (uint64, time.Time, error) {
			loadCount.Add(1)
			return 99, time.Now().Add(time.Second), nil
		},
	)

	require.NoError(t, err)
	assert.Equal(t, uint64(99), val)
	assert.Equal(t, int32(1), loadCount.Load())

	cached, expiresAt, ok := c.Peek()
	assert.True(t, ok)
	assert.Equal(t, uint64(99), cached)
	assert.True(t, expiresAt.After(time.Now()))
}

func TestPeekOrLoadHit(t *testing.T) {
	t.Parallel()
	const expectedVal = uint64(42)

	now := time.Now()
	expiresAt := now.Add(time.Second)
	c := newTestCache()
	c.Set(expectedVal, expiresAt)

	var loadCount atomic.Int32
	val, err := c.PeekOrLoad(
		context.Background(),
		now,
		func(value uint64, expiresAt, now time.Time) (bool, time.Time) {
			return now.Before(expiresAt), expiresAt
		},
		func() (uint64, time.Time, error) {
			loadCount.Add(1)
			return expectedVal, now.Add(time.Second), nil
		},
	)

	require.NoError(t, err)
	assert.Equal(t, expectedVal, val)
	assert.Equal(t, int32(0), loadCount.Load())
}

func TestPeekOrLoadError(t *testing.T) {
	t.Parallel()

	c := newTestCache()
	errLoad := errors.New("load failed")

	val, err := c.PeekOrLoad(
		context.Background(),
		time.Now(),
		func(value uint64, expiresAt, now time.Time) (bool, time.Time) {
			return false, time.Time{}
		},
		func() (uint64, time.Time, error) {
			return 0, time.Time{}, errLoad
		},
	)

	assert.ErrorIs(t, err, errLoad)
	assert.Equal(t, uint64(0), val)

	_, _, ok := c.Peek()
	assert.False(t, ok)
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
					now,
					func(value uint64, expiresAt, now time.Time) (bool, time.Time) {
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

func TestPeekOrLoadReloadAfterExpiry(t *testing.T) {
	t.Parallel()

	now := time.Now()
	c := newTestCache()
	c.Set(42, now)

	val, err := c.PeekOrLoad(
		context.Background(),
		now.Add(time.Second+time.Millisecond),
		func(value uint64, expiresAt, now time.Time) (bool, time.Time) {
			return now.Before(expiresAt), expiresAt
		},
		func() (uint64, time.Time, error) {
			return 100, now.Add(time.Second + time.Millisecond), nil
		},
	)

	require.NoError(t, err)
	assert.Equal(t, uint64(100), val)
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
				time.Now(),
				func(value uint64, expiresAt, now time.Time) (bool, time.Time) {
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
				time.Now(),
				func(value uint64, expiresAt, now time.Time) (bool, time.Time) {
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
