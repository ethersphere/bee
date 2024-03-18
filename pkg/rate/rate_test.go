// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rate_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/rate"
)

func TestRateFirstBucket(t *testing.T) {
	t.Parallel()

	windowSize := 1000 * time.Millisecond
	var r = 15

	rate := rate.New(windowSize)
	rate.SetTimeFunc(func() time.Time { return setTime(windowSize) })
	rate.Add(r)

	got := rate.Rate()
	if got != float64(r) {
		t.Fatalf("got %v, want %v", got, r)
	}
}

// // TestIgnoreOldBuckets tests that the buckets older than the most recent two buckets are ignored in rate calculation.
func TestIgnoreOldBuckets(t *testing.T) {
	t.Parallel()

	windowSize := 1000 * time.Millisecond
	var r = 100

	rate := rate.New(windowSize)

	rate.SetTimeFunc(func() time.Time { return setTime(windowSize) })
	rate.Add(10)

	// windowSize * 3 ensures that the previous bucket is ignored
	rate.SetTimeFunc(func() time.Time { return setTime(windowSize * 3) })
	rate.Add(r)

	got := rate.Rate()
	if got != float64(r) {
		t.Fatalf("got %v, want %v", got, r)
	}
}

func TestRate(t *testing.T) {
	t.Parallel()

	// windowSizeMs := 1000
	windowSize := 1000 * time.Millisecond
	const r = 100

	// tc represents the different ratios that will applied to the previous bucket in the Rate calculation
	// eg: the ratio is x, so (1 - 1/x) will be applied to the previous bucket
	for _, tc := range []struct {
		ratio int
		rate  float64
	}{
		{ratio: 2, rate: 150},
		{ratio: 4, rate: 175},
		{ratio: 5, rate: 180},
		{ratio: 10, rate: 190},
		{ratio: 100, rate: 199},
	} {

		rate := rate.New(windowSize)

		rate.SetTimeFunc(func() time.Time { return setTime(windowSize) })
		rate.Add(r)

		rate.SetTimeFunc(func() time.Time { return setTime(windowSize*2 + windowSize/time.Duration(tc.ratio)) })
		rate.Add(r)

		got := rate.Rate()
		if got != tc.rate {
			t.Fatalf("ratio %v, got %v, want %v", tc.ratio, got, tc.rate)
		}
	}
}

func setTime(ms time.Duration) time.Time {
	return time.UnixMilli(ms.Milliseconds())
}
