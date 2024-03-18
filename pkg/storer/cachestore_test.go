// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/spinlock"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
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

		t.Run("rollback", func(t *testing.T) {
			want := errors.New("dummy error")
			lstore.SetRepoStorePutHook(func(item storage.Item) error {
				if item.Namespace() == "cacheOrderIndex" {
					return want
				}
				return nil
			})
			errChunk := chunktesting.GenerateTestRandomChunk()
			have := lstore.Cache().Put(context.TODO(), errChunk)
			if !errors.Is(have, want) {
				t.Fatalf("unexpected error on cache put: want %v have %v", want, have)
			}
			haveChunk, err := lstore.Repo().ChunkStore().Has(context.TODO(), errChunk.Address())
			if err != nil {
				t.Fatalf("ChunkStore.Has(...): unexpected error: %v", err)
			}
			if haveChunk {
				t.Fatalf("unexpected chunk state: want false have %t", haveChunk)
			}
		})
	})
	t.Run("lookup", func(t *testing.T) {
		t.Run("commit", func(t *testing.T) {
			lstore.SetRepoStorePutHook(nil)
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
		t.Run("rollback", func(t *testing.T) {
			want := errors.New("dummy error")
			lstore.SetRepoStorePutHook(func(item storage.Item) error {
				if item.Namespace() == "cacheOrderIndex" {
					return want
				}
				return nil
			})
			// fail access for the first 4 chunks. This will keep the order as is
			// from the last test.
			for idx, ch := range chunks {
				if idx > 4 {
					break
				}
				_, have := lstore.Lookup().Get(context.TODO(), ch.Address())
				if !errors.Is(have, want) {
					t.Fatalf("unexpected error in cache get: want %v have %v", want, have)
				}
			}
		})
	})
	t.Run("cache chunks beyond capacity", func(t *testing.T) {
		lstore.SetRepoStorePutHook(nil)
		// add chunks beyond capacity and verify the correct chunks are removed
		// from the cache based on last access order

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
