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
	r := 15

	rate := rate.New(windowSize)
	rate.SetTimeFunc(func() time.Time { return setTime(windowSize) })
	rate.Add(r)

	got := rate.Rate()
	if got != float64(r) {
		t.Fatalf("got %v, want %v", got, r)
	}
}

// TestIgnoreOldBuckets tests that counts older than one window (rounded up to
// whole buckets) plus one bucket are ignored in rate calculation.
func TestIgnoreOldBuckets(t *testing.T) {
	t.Parallel()

	windowSize := 1000 * time.Millisecond
	bucket := (windowSize / 30).Truncate(time.Millisecond)
	covered := coveredWindow(windowSize, bucket)
	r := 100

	rate := rate.New(windowSize)

	rate.SetTimeFunc(func() time.Time { return setTime(windowSize) })
	rate.Add(10)

	rate.SetTimeFunc(func() time.Time { return setTime(windowSize + covered + bucket) })
	rate.Add(r)

	got := rate.Rate()
	if got != float64(r) {
		t.Fatalf("got %v, want %v", got, r)
	}
}

// TestRate tests that all counts within the window contribute at full weight,
// non-positive counts are ignored, and buckets left ahead of a backward clock
// jump do not count.
func TestRate(t *testing.T) {
	t.Parallel()

	windowSize := 3000 * time.Millisecond
	rate := rate.New(windowSize)

	for _, at := range []time.Duration{0, 1000 * time.Millisecond, 2500 * time.Millisecond} {
		rate.SetTimeFunc(func() time.Time { return setTime(windowSize + at) })
		rate.Add(100)
		rate.Add(0)
		rate.Add(-50)
	}

	if got, want := rate.Rate(), 100.0; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}

	rate.SetTimeFunc(func() time.Time { return setTime(windowSize - time.Second) })
	if got, want := rate.Rate(), 0.0; got != want {
		t.Fatalf("after a backward clock jump: got %v, want %v", got, want)
	}
}

// TestRateDecaysLinearly tests that the rate drops by an equal step as each
// bucket leaves the window, starting one bucket after the window has passed.
func TestRateDecaysLinearly(t *testing.T) {
	t.Parallel()

	const buckets = 30
	windowSize := 30 * time.Second
	bucket := windowSize / buckets

	rate := rate.New(windowSize)

	start := windowSize
	for i := range buckets {
		rate.SetTimeFunc(func() time.Time { return setTime(start + time.Duration(i)*bucket) })
		rate.Add(10)
	}
	last := start + (buckets-1)*bucket

	full := rate.Rate()
	if want := 10.0; full != want {
		t.Fatalf("full window: got %v, want %v", full, want)
	}

	step := full / buckets
	for i := 1; i <= buckets; i++ {
		rate.SetTimeFunc(func() time.Time { return setTime(last + time.Duration(i+1)*bucket) })
		got := rate.Rate()
		want := full - float64(i)*step
		if diff := got - want; diff > 1e-9 || diff < -1e-9 {
			t.Fatalf("%d buckets after the last add: got %v, want %v", i, got, want)
		}
	}
}

// TestRateReachesZeroWithinWindow tests that a count stays in the rate for a
// full window and is gone one window (rounded up to whole buckets) plus one
// bucket after the last Add, wherever in a bucket that Add landed. The 1s
// window does not divide into whole buckets.
func TestRateReachesZeroWithinWindow(t *testing.T) {
	t.Parallel()

	for _, windowSize := range []time.Duration{5 * time.Minute, time.Second} {
		bucket := max(windowSize/30, time.Millisecond).Truncate(time.Millisecond)
		covered := coveredWindow(windowSize, bucket)

		for _, offset := range []time.Duration{
			0,
			time.Millisecond,
			windowSize / 10,
			windowSize / 2,
			windowSize - time.Millisecond,
			// the last millisecond of a bucket, where a count lives the shortest
			bucket - windowSize%bucket - time.Millisecond,
		} {
			rate := rate.New(windowSize)
			addAt := windowSize + offset

			rate.SetTimeFunc(func() time.Time { return setTime(addAt) })
			rate.Add(1000)
			if rate.Rate() == 0 {
				t.Fatalf("window %v offset %v: rate is zero straight after an add", windowSize, offset)
			}

			rate.SetTimeFunc(func() time.Time { return setTime(addAt + windowSize - time.Millisecond) })
			if rate.Rate() == 0 {
				t.Fatalf("window %v offset %v: count expired before a full window passed", windowSize, offset)
			}

			rate.SetTimeFunc(func() time.Time { return setTime(addAt + covered + bucket) })
			if got := rate.Rate(); got != 0 {
				t.Fatalf("window %v offset %v: one covered window plus one bucket after the last add, got %v, want 0", windowSize, offset, got)
			}
		}
	}
}

func setTime(ms time.Duration) time.Time {
	return time.UnixMilli(ms.Milliseconds())
}

// coveredWindow returns the window rounded up to whole buckets.
func coveredWindow(window, bucket time.Duration) time.Duration {
	ws, bs := window.Milliseconds(), bucket.Milliseconds()
	return time.Duration((ws+bs-1)/bs*bs) * time.Millisecond
}
