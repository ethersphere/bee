// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rate_test

import (
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/rate"
)

func TestRateFirstBucket(t *testing.T) {

	var windowSize int64 = 1000
	var r = 15

	rate := rate.New(time.Millisecond * time.Duration(windowSize))
	rate.SetTimeFunc(func() int64 { return windowSize })
	rate.Add(r)

	got := rate.Rate()
	if got != float64(r) {
		t.Fatalf("got %v, want %v", got, r)
	}
}

// TestIgnoreOldBuckets tests that the buckets older than the most recent two buckets are ignored in rate calculation.
func TestIgnoreOldBuckets(t *testing.T) {

	var windowSize int64 = 1000
	var r = 100

	rate := rate.New(time.Millisecond * time.Duration(windowSize))
	rate.SetTimeFunc(func() int64 { return windowSize })
	rate.Add(10)

	// windowSize * 3 ensures that the previous bucket is ignored
	rate.SetTimeFunc(func() int64 { return windowSize * 3 })
	rate.Add(r)

	got := rate.Rate()
	if got != float64(r) {
		t.Fatalf("got %v, want %v", got, r)
	}
}

func TestRate(t *testing.T) {

	var windowSize int64 = 1000
	var r = 100

	// tc represents the different ratios that will applied to the previous bucket in the Rate calculation
	// eg: the ratio is 1/tc, so (1 - 1/tc) will be applied to the previous bucket
	for _, tc := range []int64{
		2, 4, 5, 10, 20, 100,
	} {
		rate := rate.New(time.Millisecond * time.Duration(windowSize))
		rate.SetTimeFunc(func() int64 { return windowSize })
		rate.Add(r)

		rate.SetTimeFunc(func() int64 { return 2*windowSize + windowSize/tc })
		rate.Add(r)

		got := rate.Rate()
		want := float64(r + (r - r/int(tc)))
		if got != want {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
