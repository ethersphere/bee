// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/ratelimit"
)

func TestRateLimit(t *testing.T) {
	t.Parallel()

	var (
		key1  = "test1"
		key2  = "test2"
		rate  = time.Second
		burst = 10
	)

	limiter := ratelimit.New(rate, burst)

	if !limiter.Allow(key1, burst) {
		t.Fatal("want allowed")
	}

	if limiter.Allow(key1, burst) {
		t.Fatalf("want not allowed")
	}

	limiter.Clear(key1)

	if !limiter.Allow(key1, burst) {
		t.Fatal("want allowed")
	}

	if !limiter.Allow(key2, burst) {
		t.Fatal("want allowed")
	}
}

func TestWait(t *testing.T) {
	t.Parallel()

	var (
		key   = "test"
		rate  = time.Second
		burst = 4
	)

	limiter := ratelimit.New(rate, burst)

	if !limiter.Allow(key, 1) {
		t.Fatal("want allowed")
	}

	waitDur, err := limiter.Wait(context.Background(), key, 1)
	if err != nil {
		t.Fatalf("got err %v", err)
	}

	if waitDur != 0 {
		t.Fatalf("expected the limiter to NOT wait, got %s", waitDur)
	}

	waitDur, err = limiter.Wait(context.Background(), key, burst)
	if err != nil {
		t.Fatalf("got err %v", err)
	}

	if waitDur < rate {
		t.Fatalf("expected the limiter to wait at least %s, got %s", rate, waitDur)
	}
}
