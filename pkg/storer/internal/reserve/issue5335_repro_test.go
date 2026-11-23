// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve_test

// Throwaway reproduction for https://github.com/ethersphere/bee/issues/5335.
// High-concurrency Reserve Has lookups on a disk LevelDB index (pullsync
// offer path). Most lookups are hits, matching a saturated reserve.
// Run with -cpuprofile to inspect leveldb cache/lru.Promote.

import (
	"context"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	chunk "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	kademlia "github.com/ethersphere/bee/v2/pkg/topology/mock"
	"github.com/spf13/afero"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func TestReserveHasContention(t *testing.T) {
	const (
		chunkCount   = 10_000
		workers      = 256
		opsPerWorker = 2_000
		hitPercent   = 98
	)

	baseAddr := swarm.RandAddress(t)
	ts := newDiskTransactionStorage(t)

	r, err := reserve.New(
		baseAddr,
		ts,
		chunkCount*2,
		kademlia.NewTopologyDriver(),
		log.Noop,
	)
	if err != nil {
		t.Fatal(err)
	}

	type key struct {
		addr      swarm.Address
		batchID   []byte
		stampHash []byte
	}
	keys := make([]key, 0, chunkCount)
	ctx := context.Background()

	t.Log("seeding reserve")
	seedStart := time.Now()
	for range chunkCount {
		ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0)
		if err := r.Put(ctx, ch); err != nil {
			t.Fatal(err)
		}
		stampHash, err := ch.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, key{
			addr:      ch.Address().Clone(),
			batchID:   append([]byte(nil), ch.Stamp().BatchID()...),
			stampHash: stampHash,
		})
	}
	t.Logf("seeded %d chunks in %s", chunkCount, time.Since(seedStart))

	missKeys := make([]key, 0, chunkCount/10)
	for range cap(missKeys) {
		ch := chunk.GenerateTestRandomChunkAt(t, baseAddr, 0)
		stampHash, err := ch.Stamp().Hash()
		if err != nil {
			t.Fatal(err)
		}
		missKeys = append(missKeys, key{
			addr:      ch.Address().Clone(),
			batchID:   append([]byte(nil), ch.Stamp().BatchID()...),
			stampHash: stampHash,
		})
	}

	var (
		hits     atomic.Uint64
		misses   atomic.Uint64
		errCount atomic.Uint64
	)

	t.Log("hammering Has")
	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := range workers {
		go func(worker int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(worker + 1)))
			for range opsPerWorker {
				var k key
				if rng.Intn(100) < hitPercent {
					k = keys[rng.Intn(len(keys))]
				} else {
					k = missKeys[rng.Intn(len(missKeys))]
				}

				has, err := r.Has(k.addr, k.batchID, k.stampHash)
				if err != nil {
					errCount.Add(1)
					continue
				}
				if has {
					hits.Add(1)
				} else {
					misses.Add(1)
				}
			}
		}(w)
	}
	wg.Wait()
	elapsed := time.Since(start)

	total := hits.Load() + misses.Load()
	ops := workers * opsPerWorker
	t.Logf("has ops=%d ok=%d hits=%d misses=%d errors=%d elapsed=%s rate=%.0f/s",
		ops, total, hits.Load(), misses.Load(), errCount.Load(), elapsed,
		float64(total)/elapsed.Seconds(),
	)

	if errCount.Load() > 0 {
		t.Fatalf("unexpected Has errors: %d", errCount.Load())
	}
	if total != uint64(ops) {
		t.Fatalf("expected %d completed Has calls, got %d", ops, total)
	}
	if hits.Load() < uint64(ops)*90/100 {
		t.Fatalf("expected mostly hits, got hits=%d misses=%d", hits.Load(), misses.Load())
	}
}

type reserveTestMemFS struct {
	afero.Fs
}

func (m *reserveTestMemFS) Open(path string) (fs.File, error) {
	return m.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
}

func newDiskTransactionStorage(t *testing.T) transaction.Storage {
	t.Helper()

	dir := t.TempDir()
	store, _, err := leveldbstore.New(filepath.Join(dir, "indexstore"), &opt.Options{
		// Small cache keeps Promote on a hot path under concurrent Has.
		BlockCacheCapacity: 2 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}

	sh, err := sharky.New(&reserveTestMemFS{Fs: afero.NewMemMapFs()}, 4, swarm.SocMaxChunkSize)
	if err != nil {
		t.Fatal(err)
	}

	st := transaction.NewStorage(sh, store)
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Errorf("storage close: %v", err)
		}
	})
	return st
}
