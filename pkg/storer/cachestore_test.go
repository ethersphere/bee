// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/spinlock"
	"github.com/ethersphere/bee/v2/pkg/storage/storagetest"
	chunktesting "github.com/ethersphere/bee/v2/pkg/storage/testing"
	storer "github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func testCacheStore(t *testing.T, newStorer func() (*storer.DB, error)) {
	t.Helper()

	chunks := chunktesting.GenerateTestRandomChunks(9)

	lstore, err := newStorer()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("cache chunks", func(t *testing.T) {
		t.Run("commit", func(t *testing.T) {
			putter := lstore.Cache()
			for _, ch := range chunks {
				err := putter.Put(context.TODO(), ch)
				if err != nil {
					t.Fatalf("Cache.Put(...): unexpected error: %v", err)
				}
			}
		})
	})
	t.Run("lookup", func(t *testing.T) {
		t.Run("commit", func(t *testing.T) {
			getter := lstore.Lookup()
			for _, ch := range chunks {
				have, err := getter.Get(context.TODO(), ch.Address())
				if err != nil {
					t.Fatalf("Cache.Get(...): unexpected error: %v", err)
				}
				if !have.Equal(ch) {
					t.Fatalf("chunk %s does not match", ch.Address())
				}
			}
		})
	})
	t.Run("cache chunks beyond capacity", func(t *testing.T) {
		newChunks := chunktesting.GenerateTestRandomChunks(5)
		putter := lstore.Cache()
		for _, ch := range newChunks {
			err := putter.Put(context.TODO(), ch)
			if err != nil {
				t.Fatalf("Cache.Put(...): unexpected error: %v", err)
			}
		}

		info, err := lstore.DebugInfo(context.Background())
		if err != nil {
			t.Fatal(err)
		}

		info, err = lstore.DebugInfo(context.Background())
		if err != nil {
			t.Fatal(err)
		}

		err = spinlock.WaitWithInterval(time.Second*5, time.Second, func() bool {
			info, err = lstore.DebugInfo(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if info.Cache.Size == 10 {
				return true
			}
			return false
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestCacheStore(t *testing.T) {
	t.Parallel()

	t.Run("inmem", func(t *testing.T) {
		t.Parallel()

		testCacheStore(t, func() (*storer.DB, error) {

			opts := dbTestOps(swarm.RandAddress(t), 100, nil, nil, time.Second)
			opts.CacheCapacity = 10

			return storer.New(context.Background(), "", opts)
		})
	})
	t.Run("disk", func(t *testing.T) {
		t.Parallel()

		opts := dbTestOps(swarm.RandAddress(t), 100, nil, nil, time.Second)
		opts.CacheCapacity = 10

		testCacheStore(t, diskStorer(t, opts))
	})
}

func BenchmarkCachePutter(b *testing.B) {
	baseAddr := swarm.RandAddress(b)
	opts := dbTestOps(baseAddr, 10000, nil, nil, time.Second)
	opts.CacheCapacity = 10
	storer, err := diskStorer(b, opts)()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	storagetest.BenchmarkChunkStoreWriteSequential(b, storer.Cache())
}
