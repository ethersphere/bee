// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldbstore_test

import (
	"fmt"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	"github.com/ethersphere/bee/v2/pkg/storage/storageutil"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// probeItem is a small item used to fill LevelDB tables with data blocks.
type probeItem struct {
	id   string
	data []byte
}

func (p *probeItem) ID() string               { return p.id }
func (p *probeItem) Namespace() string        { return "probe" }
func (p *probeItem) Marshal() ([]byte, error) { return p.data, nil }
func (p *probeItem) String() string           { return storageutil.JoinFields(p.Namespace(), p.ID()) }

func (p *probeItem) Unmarshal(b []byte) error {
	p.data = append([]byte(nil), b...)
	return nil
}

func (p *probeItem) Clone() storage.Item {
	return &probeItem{id: p.id, data: append([]byte(nil), p.data...)}
}

func probePayload(i int) string { return fmt.Sprintf("payload-%08d-%064d", i, i) }

// TestReaderWithOptionsDontFillCache proves that point reads made through a
// reader derived with storage.WithDontFillCache leave the block cache as it
// is, while the same reads made through the store itself populate it.
func TestReaderWithOptionsDontFillCache(t *testing.T) {
	t.Parallel()

	st, _, err := leveldbstore.New(t.TempDir(), &opt.Options{
		BlockCacheCapacity:  1024 * 1024,
		BlockSize:           1024,
		CompactionTableSize: 64 * 1024,
		Filter:              filter.NewBloomFilter(10),
	})
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	const numItems = 2000
	for i := range numItems {
		err := st.Put(&probeItem{id: fmt.Sprintf("%08d", i), data: []byte(probePayload(i))})
		if err != nil {
			t.Fatalf("put: %v", err)
		}
	}
	// Flush the memtable into tables so that reads go through the block cache.
	if err := st.DB().CompactRange(util.Range{}); err != nil {
		t.Fatalf("compact: %v", err)
	}

	blockCacheSize := func() int {
		t.Helper()
		var stats leveldb.DBStats
		if err := st.DB().Stats(&stats); err != nil {
			t.Fatalf("stats: %v", err)
		}
		return stats.BlockCacheSize
	}

	readAll := func(r storage.Reader) {
		t.Helper()
		for i := range numItems {
			item := &probeItem{id: fmt.Sprintf("%08d", i)}
			has, err := r.Has(item)
			if err != nil {
				t.Fatalf("has %s: %v", item.id, err)
			}
			if !has {
				t.Fatalf("has %s: item missing", item.id)
			}
			if err := r.Get(item); err != nil {
				t.Fatalf("get %s: %v", item.id, err)
			}
			if want := probePayload(i); string(item.data) != want {
				t.Fatalf("get %s: have %q want %q", item.id, item.data, want)
			}
		}
	}

	noFill := st.ReaderWithOptions(storage.WithDontFillCache())

	// The first pass still caches the index and filter blocks of every table,
	// the second pass must not add anything on top of that.
	readAll(noFill)
	afterFirstPass := blockCacheSize()
	readAll(noFill)
	afterSecondPass := blockCacheSize()
	if afterSecondPass != afterFirstPass {
		t.Fatalf("no-fill reads changed the block cache size: %d -> %d", afterFirstPass, afterSecondPass)
	}

	readAll(st)
	afterFillPass := blockCacheSize()
	if afterFillPass <= afterSecondPass {
		t.Fatalf("plain reads did not add data blocks to the block cache: %d -> %d", afterSecondPass, afterFillPass)
	}
}
