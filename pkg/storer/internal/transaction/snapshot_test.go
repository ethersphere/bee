// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction_test

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand"
	"slices"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// snapshotItem is a minimal storage.Item with an arbitrary ID and value.
type snapshotItem struct {
	id  string
	val []byte
}

func (s *snapshotItem) ID() string        { return s.id }
func (s *snapshotItem) Namespace() string { return "snapshotItem" }
func (s *snapshotItem) Marshal() ([]byte, error) {
	return slices.Clone(s.val), nil
}

func (s *snapshotItem) Unmarshal(buf []byte) error {
	s.val = slices.Clone(buf)
	return nil
}

func (s *snapshotItem) Clone() storage.Item {
	return &snapshotItem{id: s.id, val: slices.Clone(s.val)}
}
func (s *snapshotItem) String() string { return s.Namespace() + "/" + s.id }

type kv struct {
	key string
	val string
}

// TestIndexStoreIterateSnapshot checks that an iteration over the IndexStore of
// transaction.Storage, backed by leveldbstore as the storer constructs it, reads
// a consistent snapshot taken when the iteration starts: transactions that add,
// delete and overwrite keys in the iterated namespace while the iterator is open
// are not seen, neither in front of nor behind the cursor.
//
// The fingerprint index rebuild of the rangesync proposal (§5.2) relies on this.
// The leveldb write buffer is tiny so that memtable flushes and table compactions
// run while the iterator is open.
func TestIndexStoreIterateSnapshot(t *testing.T) {
	t.Parallel()

	const (
		initial    = 2000
		txPerWave  = 20
		opsPerTx   = 10
		middleItem = initial / 2
	)

	queries := []struct {
		name  string
		query storage.Query
	}{
		{
			name:  "namespace ascending",
			query: storage.Query{Factory: func() storage.Item { return new(snapshotItem) }},
		},
		{
			name: "prefix at start",
			query: storage.Query{
				Factory:       func() storage.Item { return new(snapshotItem) },
				Prefix:        snapshotKey(initial / 4),
				PrefixAtStart: true,
			},
		},
		{
			name: "namespace descending",
			query: storage.Query{
				Factory: func() storage.Item { return new(snapshotItem) },
				Order:   storage.KeyDescendingOrder,
			},
		},
	}

	for _, tc := range queries {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			st, ldb := newDiskStorage(t)
			rnd := rand.New(rand.NewSource(1))

			// Even key numbers exist before the iteration; odd ones are free for
			// inserts in between existing keys while the iterator is open.
			live := make(map[int]bool)
			err := st.Run(context.Background(), func(s transaction.Store) error {
				for i := range initial {
					n := 2 * i
					live[n] = true
					if err := s.IndexStore().Put(&snapshotItem{id: snapshotKey(n), val: snapshotValue(n, 0)}); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}

			before := collect(t, st, tc.query, nil)
			if len(before) == 0 {
				t.Fatal("empty pre-state")
			}

			wave := 0
			commitWave := func() {
				wave++
				for range txPerWave {
					err := st.Run(context.Background(), func(s transaction.Store) error {
						for range opsPerTx {
							n := rnd.Intn(4 * initial)
							switch {
							case live[n] && rnd.Intn(2) == 0:
								delete(live, n)
								if err := s.IndexStore().Delete(&snapshotItem{id: snapshotKey(n)}); err != nil {
									return err
								}
							default:
								// insert a new key or overwrite an existing one
								live[n] = true
								if err := s.IndexStore().Put(&snapshotItem{id: snapshotKey(n), val: snapshotValue(n, wave)}); err != nil {
									return err
								}
							}
						}
						return nil
					})
					if err != nil {
						t.Fatal(err)
					}
				}
			}

			memCompBefore := memCompactions(t, ldb)

			// Commit from inside the callback: the iterator is open, and the
			// commits land both in front of and behind the cursor.
			during := collect(t, st, tc.query, func(i int) {
				if i == 0 || i == middleItem {
					commitWave()
				}
			})
			if wave != 2 {
				t.Fatalf("committed %d waves during the iteration, want 2", wave)
			}
			// The commits must have flushed memtables to tables while the
			// iterator was open, otherwise the test only covers the memtable.
			if n := memCompactions(t, ldb) - memCompBefore; n == 0 {
				t.Fatal("no memtable compaction during the iteration")
			}

			if !slices.Equal(before, during) {
				t.Fatalf("iteration is not a snapshot: got %d items, want %d (the pre-open state)", len(during), len(before))
			}

			// The commits really landed: a new iteration sees them.
			after := collect(t, st, tc.query, nil)
			if slices.Equal(before, after) {
				t.Fatal("concurrent commits are not visible to a new iteration")
			}
		})
	}
}

// collect runs one iteration and returns its items in iteration order. hook is
// called with the item's position before the item is recorded.
func collect(t *testing.T, st transaction.Storage, q storage.Query, hook func(int)) []kv {
	t.Helper()

	var out []kv
	err := st.IndexStore().Iterate(q, func(r storage.Result) (bool, error) {
		if hook != nil {
			hook(len(out))
		}
		it := r.Entry.(*snapshotItem)
		out = append(out, kv{key: r.ID, val: string(it.val)})
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// newDiskStorage returns a transaction.Storage over an on-disk leveldbstore,
// as initDiskRepository in pkg/storer builds it, with a tiny write buffer.
func newDiskStorage(t *testing.T) (transaction.Storage, *leveldbstore.Store) {
	t.Helper()

	sharkyStore, err := sharky.New(&dirFS{basedir: t.TempDir()}, 1, swarm.SocMaxChunkSize)
	if err != nil {
		t.Fatal(err)
	}
	store, _, err := leveldbstore.New(t.TempDir(), &opt.Options{
		WriteBuffer:         16 * 1024,
		CompactionL0Trigger: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	st := transaction.NewStorage(sharkyStore, store)
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Error(err)
		}
	})
	return st, store
}

func memCompactions(t *testing.T, store *leveldbstore.Store) uint32 {
	t.Helper()

	var stats leveldb.DBStats
	if err := store.DB().Stats(&stats); err != nil {
		t.Fatal(err)
	}
	return stats.MemComp
}

func snapshotKey(n int) string { return fmt.Sprintf("%08d", n) }

func snapshotValue(n, wave int) []byte {
	// large values fill the 16 KiB write buffer quickly
	b := make([]byte, 1024)
	binary.BigEndian.PutUint64(b, uint64(n))
	binary.BigEndian.PutUint64(b[8:], uint64(wave))
	return b
}
