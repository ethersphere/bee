// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/transaction/backendmock"
	"github.com/ethersphere/bee/pkg/transaction/cache"
)

func TestNew(t *testing.T) {
	t.Parallel()

	now := time.Now()
	ctx := context.Background()
	blockNo := uint64(1)
	backendCalls := 0
	backend := backendmock.New(backendmock.WithBlockNumberFunc(func(ctx context.Context) (uint64, error) {
		backendCalls++
		return blockNo, nil
	}))

	backendCached := cache.NewWithOptions(backend, func() time.Time { return now })

	for i := 0; i < 100; i++ {
		for j := 0; j < 10; j++ {
			bn, _ := backendCached.BlockNumber(ctx)
			if bn != blockNo {
				t.Fatalf("block number not as expected: want %d, got %d", blockNo, bn)
			}
		}

		if backendCalls != i+1 {
			t.Fatalf("backend calls not as expected: want %d, got %d", i+1, backendCalls)
		}

		now = now.Add(cache.DefaultBlockNumberCacheInterval).Add(time.Nanosecond)
		blockNo++
	}
}
