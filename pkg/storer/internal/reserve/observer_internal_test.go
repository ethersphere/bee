// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// callStorage runs the callback without a store and allocates nothing.
type callStorage struct {
	transaction.Storage
}

func (callStorage) Run(_ context.Context, f func(transaction.Store) error) error { return f(nil) }

// TestRunDisabledAllocs checks that with the address index disabled run adds
// no allocation to the plain r.st.Run call it replaces: the default path is
// unchanged.
func TestRunDisabledAllocs(t *testing.T) {
	r := &Reserve{st: callStorage{}}
	ctx := context.Background()

	var calls int
	f := func(transaction.Store) error { calls++; return nil }
	direct := testing.AllocsPerRun(100, func() { _ = r.st.Run(ctx, f) })
	wrapped := testing.AllocsPerRun(100, func() { _ = r.run(ctx, r.newAddrTx(), f) })
	if wrapped != direct {
		t.Fatalf("prebuilt callback: run allocates %v per call, r.st.Run %v", wrapped, direct)
	}

	// The call shape of the reserve: a closure built per transaction that
	// captures the caller's state (and, after the change, the nil writer).
	item := &BatchRadiusItem{Address: swarm.RandAddress(t)}
	direct = testing.AllocsPerRun(100, func() {
		_ = r.st.Run(ctx, func(transaction.Store) error {
			calls += len(item.Address.Bytes())
			return nil
		})
	})
	wrapped = testing.AllocsPerRun(100, func() {
		tx := r.newAddrTx()
		_ = r.run(ctx, tx, func(transaction.Store) error {
			calls += len(item.Address.Bytes())
			return tx.delete(nil, item.Address, item.StampHash, item.BatchID)
		})
	})
	if wrapped != direct {
		t.Fatalf("per-call closure: run allocates %v per call, r.st.Run %v", wrapped, direct)
	}
	if calls == 0 {
		t.Fatal("callbacks did not run")
	}
}

type internalDirFS struct {
	basedir string
}

func (d *internalDirFS) Open(path string) (fs.File, error) {
	return os.OpenFile(filepath.Join(d.basedir, path), os.O_RDWR|os.O_CREATE, 0o644)
}

func newInternalDiskStorage(t *testing.T) transaction.Storage {
	t.Helper()

	sharkyStore, err := sharky.New(&internalDirFS{basedir: t.TempDir()}, 1, swarm.SocMaxChunkSize)
	if err != nil {
		t.Fatal(err)
	}
	store, _, err := leveldbstore.New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	st := transaction.NewStorage(sharkyStore, store)
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Error(err)
		}
	})
	return st
}

// TestAddrTxSameKeyTwice checks the delta of a transaction that touches one
// key twice. The transaction's reads see the committed state, not its own
// pending writes, so the writer tracks the keys it wrote or deleted.
func TestAddrTxSameKeyTwice(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st := newInternalDiskStorage(t)

	id := func() ChunkID {
		return ChunkID{Address: swarm.RandAddress(t), StampHash: swarm.RandAddress(t).Bytes(), BatchID: swarm.RandAddress(t).Bytes()}
	}
	key := func(ids []ChunkID) []string {
		out := make([]string, 0, len(ids))
		for _, id := range ids {
			out = append(out, id.Address.ByteString()+string(id.StampHash))
		}
		slices.Sort(out)
		return out
	}
	existing := id()
	err := st.Run(ctx, func(s transaction.Store) error {
		return (&addrTx{}).put(s.IndexStore(), existing.Address, existing.StampHash, existing.BatchID, swarm.ChunkTypeContentAddressed)
	})
	if err != nil {
		t.Fatal(err)
	}
	fresh := id()

	tests := []struct {
		name          string
		ops           func(tx *addrTx, s transaction.Store) error
		adds, removes []ChunkID
	}{
		{
			name: "delete then put an existing id",
			ops: func(tx *addrTx, s transaction.Store) error {
				if err := tx.delete(s.IndexStore(), existing.Address, existing.StampHash, existing.BatchID); err != nil {
					return err
				}
				return tx.put(s.IndexStore(), existing.Address, existing.StampHash, existing.BatchID, swarm.ChunkTypeContentAddressed)
			},
			adds:    []ChunkID{existing},
			removes: []ChunkID{existing},
		},
		{
			name: "put then delete a new id",
			ops: func(tx *addrTx, s transaction.Store) error {
				if err := tx.put(s.IndexStore(), fresh.Address, fresh.StampHash, fresh.BatchID, swarm.ChunkTypeContentAddressed); err != nil {
					return err
				}
				return tx.delete(s.IndexStore(), fresh.Address, fresh.StampHash, fresh.BatchID)
			},
			adds:    []ChunkID{fresh},
			removes: []ChunkID{fresh},
		},
		{
			name: "delete an existing id twice",
			ops: func(tx *addrTx, s transaction.Store) error {
				for range 2 {
					if err := tx.delete(s.IndexStore(), existing.Address, existing.StampHash, existing.BatchID); err != nil {
						return err
					}
				}
				return nil
			},
			removes: []ChunkID{existing},
		},
		{
			name: "put a new id twice",
			ops: func(tx *addrTx, s transaction.Store) error {
				for range 2 {
					if err := tx.put(s.IndexStore(), fresh.Address, fresh.StampHash, fresh.BatchID, swarm.ChunkTypeContentAddressed); err != nil {
						return err
					}
				}
				return nil
			},
			adds: []ChunkID{fresh},
		},
	}
	for _, tc := range tests {
		// Each case runs in its own transaction that is never committed, so
		// every case starts from the same committed state.
		tx := &addrTx{record: true}
		trx, done := st.NewTransaction(ctx)
		err := tc.ops(tx, trx)
		done()
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !slices.Equal(key(tx.adds), key(tc.adds)) || !slices.Equal(key(tx.removes), key(tc.removes)) {
			t.Fatalf("%s: %d adds, %d removes; want %d, %d", tc.name, len(tx.adds), len(tx.removes), len(tc.adds), len(tc.removes))
		}
	}
}
