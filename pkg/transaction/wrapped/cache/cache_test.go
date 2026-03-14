// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testKey Key = "test"

func newTestCache(ttl time.Duration) *ExpiringSingleFlightCache[uint64] {
	return &ExpiringSingleFlightCache[uint64]{
		ttl:     ttl,
		key:     testKey,
		metrics: newMetricSet(string(testKey)),
	}
}

func TestSetGet(t *testing.T) {
	t.Parallel()

	now := time.Now()
	c := newTestCache(time.Second)
	c.Set(42, now)

	val, ok := c.Get(now)
	assert.True(t, ok)
	assert.Equal(t, uint64(42), val)
}

func TestTTLExpiry(t *testing.T) {
	t.Parallel()

	now := time.Now()
	c := newTestCache(time.Second)
	c.Set(42, now)

	_, ok := c.Get(now.Add(time.Second + time.Millisecond))
	assert.False(t, ok)
}

func TestInvalidate(t *testing.T) {
	t.Parallel()

	now := time.Now()
	c := newTestCache(5 * time.Second)
	c.Set(42, now)
	c.Invalidate()

	_, ok := c.Get(now)
	assert.False(t, ok)
}

func TestGetOrLoadMiss(t *testing.T) {
	t.Parallel()

	c := newTestCache(time.Second)
	var loadCount atomic.Int32

	val, err := c.GetOrLoad(time.Now(), func() (uint64, error) {
		loadCount.Add(1)
		return 99, nil
	})

	require.NoError(t, err)
	assert.Equal(t, uint64(99), val)
	assert.Equal(t, int32(1), loadCount.Load())

	cached, ok := c.Get(time.Now())
	assert.True(t, ok)
	assert.Equal(t, uint64(99), cached)
}

func TestGetOrLoadHit(t *testing.T) {
	t.Parallel()

	now := time.Now()
	c := newTestCache(time.Second)
	c.Set(42, now)

	var loadCount atomic.Int32
	val, err := c.GetOrLoad(now, func() (uint64, error) {
		loadCount.Add(1)
		return 99, nil
	})

	require.NoError(t, err)
	assert.Equal(t, uint64(42), val)
	assert.Equal(t, int32(0), loadCount.Load())
}

func TestGetOrLoadError(t *testing.T) {
	t.Parallel()

	c := newTestCache(time.Second)
	errLoad := errors.New("load failed")

	val, err := c.GetOrLoad(time.Now(), func() (uint64, error) {
		return 0, errLoad
	})

	assert.ErrorIs(t, err, errLoad)
	assert.Equal(t, uint64(0), val)

	_, ok := c.Get(time.Now())
	assert.False(t, ok)
}

func TestGetOrLoadSingleflight(t *testing.T) {
	t.Parallel()

	c := newTestCache(5 * time.Second)
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
			results[idx], errs[idx] = c.GetOrLoad(time.Now(), func() (uint64, error) {
				loadCount.Add(1)
				<-gate
				return 77, nil
			})
		}(i)
	}

	time.Sleep(50 * time.Millisecond)
	close(gate)
	wg.Wait()

	assert.Equal(t, int32(1), loadCount.Load())
	for i := range n {
		assert.NoError(t, errs[i])
		assert.Equal(t, uint64(77), results[i])
	}
}

func TestGetOrLoadReloadAfterExpiry(t *testing.T) {
	t.Parallel()

	now := time.Now()
	c := newTestCache(time.Second)
	c.Set(42, now)

	val, err := c.GetOrLoad(now.Add(time.Second+time.Millisecond), func() (uint64, error) {
		return 100, nil
	})

	require.NoError(t, err)
	assert.Equal(t, uint64(100), val)
}

func TestMetrics(t *testing.T) {
	t.Parallel()

	now := time.Now()
	c := newTestCache(time.Second)

	// miss + load
	_, _ = c.GetOrLoad(now, func() (uint64, error) { return 42, nil })
	// hit
	_, _ = c.GetOrLoad(now, func() (uint64, error) { return 0, nil })
	// invalidate
	c.Invalidate()
	// miss + load error
	_, _ = c.GetOrLoad(now, func() (uint64, error) { return 0, errors.New("fail") })

	assert.Equal(t, float64(1), testutil.ToFloat64(c.metrics.Hits))
	assert.Equal(t, float64(2), testutil.ToFloat64(c.metrics.Misses))
	assert.Equal(t, float64(2), testutil.ToFloat64(c.metrics.Loads))
	assert.Equal(t, float64(1), testutil.ToFloat64(c.metrics.LoadErrors))
	assert.Equal(t, float64(1), testutil.ToFloat64(c.metrics.Invalidates))
}
