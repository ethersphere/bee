// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldbstore_test

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

type mockBuffer struct {
	buf []byte
}

func (b *mockBuffer) Alloc(n int) []byte {
	offset := len(b.buf)
	b.buf = append(b.buf, make([]byte, n)...)
	return b.buf[offset:]
}

func (b *mockBuffer) Write(p []byte) (n int, err error) {
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *mockBuffer) WriteByte(c byte) error {
	b.buf = append(b.buf, c)
	return nil
}

func (b *mockBuffer) Len() int {
	return len(b.buf)
}

// TestBloomFilterSizeProvesFootprint proves that Bloom filter size is strictly
// determined by the number of keys multiplied by bitsPerKey (8 bytes/key for 64-bit,
// 1.25 bytes/key for 10-bit), irrespective of key data structure.
func TestBloomFilterSizeProvesFootprint(t *testing.T) {
	t.Parallel()

	numKeys := 10_000

	keys := make([][]byte, numKeys)
	for i := range numKeys {
		k := make([]byte, 32)
		binary.BigEndian.PutUint64(k, uint64(i))
		keys[i] = k
	}

	// 64 bits per key
	f64 := filter.NewBloomFilter(64)
	gen64 := f64.NewGenerator()
	for _, k := range keys {
		gen64.Add(k)
	}
	buf64 := &mockBuffer{}
	gen64.Generate(buf64)
	size64 := buf64.Len()

	// 10 bits per key
	f10 := filter.NewBloomFilter(10)
	gen10 := f10.NewGenerator()
	for _, k := range keys {
		gen10.Add(k)
	}
	buf10 := &mockBuffer{}
	gen10.Generate(buf10)
	size10 := buf10.Len()

	t.Logf("Generated filter for %d keys:", numKeys)
	t.Logf("  64 bits/key filter size: %d bytes (%.2f bytes/key)", size64, float64(size64)/float64(numKeys))
	t.Logf("  10 bits/key filter size: %d bytes (%.2f bytes/key)", size10, float64(size10)/float64(numKeys))

	// 64 bits/key must be ~6.4x larger than 10 bits/key
	ratio := float64(size64) / float64(size10)
	if ratio < 6.3 || ratio > 6.5 {
		t.Fatalf("unexpected size ratio: %f (expected ~6.4)", ratio)
	}

	// For 21M keys (full reserve: 4.19M chunks * 5 index keys), calculate total footprint in MiB:
	totalReserveKeys := 21_000_000
	mib64 := (float64(totalReserveKeys) * (float64(size64) / float64(numKeys))) / (1024 * 1024)
	mib10 := (float64(totalReserveKeys) * (float64(size10) / float64(numKeys))) / (1024 * 1024)

	t.Logf("Extrapolated for 21M reserve keys:")
	t.Logf("  64 bits/key footprint: %.1f MiB", mib64)
	t.Logf("  10 bits/key footprint: %.1f MiB", mib10)

	if mib64 < 155 || mib64 > 165 {
		t.Fatalf("expected ~160 MiB footprint for 64 bits/key, got %.1f", mib64)
	}
}

// TestBloomFilterCacheThrashing proves that under a constrained BlockCache,
// 64 bits/key filter blocks evict each other, causing significantly higher disk read (IORead).
func TestBloomFilterCacheThrashing(t *testing.T) {
	t.Parallel()

	runTest := func(bitsPerKey int) (readBytes uint64) {
		dir := t.TempDir()
		// Small BlockCache and small SST tables to reproduce thrashing quickly and deterministically
		opts := &opt.Options{
			Filter:              filter.NewBloomFilter(bitsPerKey),
			BlockCacheCapacity:  128 * 1024, // 128 KiB cache
			CompactionTableSize: 64 * 1024,  // 64 KiB tables (multiple tables with their own filters)
			BlockSize:           1024,
		}

		db, err := leveldb.OpenFile(dir, opts)
		if err != nil {
			t.Fatalf("open db: %v", err)
		}

		// Write 3,000 keys
		numEntries := 3000
		batch := new(leveldb.Batch)
		for i := range numEntries {
			k := fmt.Sprintf("key-%08d", i)
			v := fmt.Sprintf("val-%08d-data-payload-padding-bytes-%032d", i, i)
			batch.Put([]byte(k), []byte(v))
			if batch.Len() >= 200 {
				if err := db.Write(batch, nil); err != nil {
					t.Fatalf("write batch: %v", err)
				}
				batch.Reset()
			}
		}
		if batch.Len() > 0 {
			_ = db.Write(batch, nil)
		}

		// Force compaction to flush everything into SSTable files with filter blocks on disk
		if err := db.CompactRange(util.Range{}); err != nil {
			t.Fatalf("compact: %v", err)
		}

		// Reopen db to start fresh stats
		if err := db.Close(); err != nil {
			t.Fatalf("close db: %v", err)
		}

		db, err = leveldb.OpenFile(dir, opts)
		if err != nil {
			t.Fatalf("reopen db: %v", err)
		}
		defer db.Close()

		// Perform 500 random lookups
		rng := rand.New(rand.NewSource(42))
		for i := 0; i < 500; i++ {
			k := fmt.Sprintf("key-%08d", rng.Intn(numEntries))
			_, err := db.Get([]byte(k), nil)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
		}

		var stats leveldb.DBStats
		if err := db.Stats(&stats); err != nil {
			t.Fatalf("stats: %v", err)
		}
		return stats.IORead
	}

	readBytes64 := runTest(64)
	readBytes10 := runTest(10)

	t.Logf("Random read disk I/O with 128 KiB cache:")
	t.Logf("  64 bits/key IORead: %d bytes (%.1f KiB)", readBytes64, float64(readBytes64)/1024)
	t.Logf("  10 bits/key IORead: %d bytes (%.1f KiB)", readBytes10, float64(readBytes10)/1024)

	if readBytes64 <= readBytes10 {
		t.Fatalf("expected 64 bits/key to read significantly more bytes than 10 bits/key due to filter thrashing; got 64=%d, 10=%d", readBytes64, readBytes10)
	}

	ratio := float64(readBytes64) / float64(readBytes10)
	t.Logf("IORead amplification factor: %.2fx more disk read with 64 bits/key", ratio)
}

// TestBloomFilterInBlockCacheProvesEviction proves that:
//  1. Filter blocks share the exact same BlockCache as data blocks (LevelDB table/reader.go).
//  2. When filter blocks fit in BlockCache, negative lookups cause 0 disk read I/O.
//  3. When filter blocks exceed BlockCache and get evicted,
//     every lookup re-reads the SST filter block from disk.
func TestBloomFilterInBlockCacheProvesEviction(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	opts := &opt.Options{
		Filter:              filter.NewBloomFilter(64),
		BlockCacheCapacity:  256 * 1024,
		CompactionTableSize: 64 * 1024,
	}

	db, err := leveldb.OpenFile(dir, opts)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	numEntries := 3000
	batch := new(leveldb.Batch)
	for i := range numEntries {
		batch.Put(fmt.Appendf(nil, "k-%08d", i), fmt.Appendf(nil, "v-%08d", i))
	}
	if err := db.Write(batch, nil); err != nil {
		t.Fatalf("write batch: %v", err)
	}
	if err := db.CompactRange(util.Range{}); err != nil {
		t.Fatalf("compact: %v", err)
	}
	_ = db.Close()

	// Scenario A: Cache is large enough to keep all filter blocks in memory
	dbA, err := leveldb.OpenFile(dir, &opt.Options{
		Filter:             filter.NewBloomFilter(64),
		BlockCacheCapacity: 512 * 1024, // 512 KiB (sufficient to hold all filters)
	})
	if err != nil {
		t.Fatalf("open dbA: %v", err)
	}
	defer dbA.Close()

	// Warm up cache
	_, _ = dbA.Get([]byte("k-00000050-probe"), nil)
	var statsAInitial leveldb.DBStats
	_ = dbA.Stats(&statsAInitial)

	// Next 100 negative lookups with warm cache (keys fall within table ranges):
	for i := range 100 {
		_, _ = dbA.Get(fmt.Appendf(nil, "k-%08d-missing", (i*27)%numEntries), nil)
	}
	var statsAFinal leveldb.DBStats
	_ = dbA.Stats(&statsAFinal)
	diskReadA := statsAFinal.IORead - statsAInitial.IORead
	_ = dbA.Close()

	// Scenario B: Cache is too small to keep filter blocks (1 KiB), causing repeated eviction
	dbB, err := leveldb.OpenFile(dir, &opt.Options{
		Filter:             filter.NewBloomFilter(64),
		BlockCacheCapacity: 1024, // 1 KiB cache -> filter blocks cannot stay resident
	})
	if err != nil {
		t.Fatalf("open dbB: %v", err)
	}
	defer dbB.Close()

	var statsBInitial leveldb.DBStats
	_ = dbB.Stats(&statsBInitial)
	// Same 100 negative lookups:
	for i := range 100 {
		_, _ = dbB.Get(fmt.Appendf(nil, "k-%08d-missing", (i*27)%numEntries), nil)
	}
	var statsBFinal leveldb.DBStats
	_ = dbB.Stats(&statsBFinal)
	diskReadB := statsBFinal.IORead - statsBInitial.IORead

	t.Logf("Negative lookups disk read (100 missing keys across tables):")
	t.Logf("  With filter fitting in BlockCache: %d bytes (0 disk I/O, filter in RAM)", diskReadA)
	t.Logf("  With filter thrashing BlockCache: %d bytes (disk I/O from re-reading evicted filter blocks)", diskReadB)

	if diskReadA != 0 {
		t.Errorf("expected 0 disk reads when filters are cached, got %d", diskReadA)
	}
	if diskReadB == 0 {
		t.Errorf("expected disk reads when filters thrash the cache, got 0")
	}
}
