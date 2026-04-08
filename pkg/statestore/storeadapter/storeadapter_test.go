// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storeadapter_test

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/bigint"
	"github.com/ethersphere/bee/v2/pkg/statestore/storeadapter"
	"github.com/ethersphere/bee/v2/pkg/statestore/test"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemstore"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
)

// TestMigrateBigIntKeys verifies that balance keys stored in legacy formats
// (unquoted decimal from json.Marshal(*big.Int) and quoted string from
// json.Marshal(bigint.BigInt)) are correctly rewritten to Gob encoding by
// migration step 9.
// rawKeyItem is a storage.Item that writes/reads raw bytes under the "ss"
// namespace used by StateStorerAdapter, allowing tests to seed legacy data
// directly into the underlying store without going through the adapter.
type rawKeyItem struct {
	key  string
	data []byte
}

func (r *rawKeyItem) ID() string               { return r.key }
func (r *rawKeyItem) Namespace() string        { return "ss" }
func (r *rawKeyItem) Marshal() ([]byte, error) { return r.data, nil }
func (r *rawKeyItem) Unmarshal(b []byte) error { r.data = append(r.data[:0], b...); return nil }
func (r *rawKeyItem) Clone() storage.Item {
	return &rawKeyItem{key: r.key, data: append([]byte{}, r.data...)}
}
func (r rawKeyItem) String() string { return "ss/" + r.key }

func TestMigrateBigIntKeys(t *testing.T) {
	t.Parallel()

	prefixes := []string{
		"accounting_balance_",
		"accounting_surplusbalance_",
		"accounting_originatedbalance_",
		"swap_chequebook_total_issued_",
	}

	type legacyEntry struct {
		key  string
		data []byte
		want *big.Int
	}

	entries := make([]legacyEntry, 0, len(prefixes))
	for _, prefix := range prefixes {
		val := big.NewInt(123456)
		// json.Marshal(*big.Int) produces an unquoted decimal number e.g. 123456
		unquoted, err := json.Marshal(val)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, legacyEntry{prefix + "peer1", unquoted, val})
	}

	// write legacy data directly into a raw store (bypassing migration)
	rawStore := inmemstore.New()
	for _, e := range entries {
		if err := rawStore.Put(&rawKeyItem{key: e.key, data: e.data}); err != nil {
			t.Fatalf("seeding key %q: %v", e.key, err)
		}
	}

	// NewStateStorerAdapter runs all migrations including step 9
	store, err := storeadapter.NewStateStorerAdapter(rawStore)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	for _, e := range entries {
		var w bigint.BigInt
		if err := store.Get(e.key, &w); err != nil {
			t.Fatalf("Get(%q): %v", e.key, err)
		}
		if w.Cmp(e.want) != 0 {
			t.Fatalf("key %q: got %v, want %v", e.key, w.Int, e.want)
		}
	}
}

// TestMigrateBigIntKeys_GobRejected verifies that migration step 9 returns an
// error if it encounters already Gob-encoded data, since this indicates an
// unexpected database state.
func TestMigrateBigIntKeys_GobRejected(t *testing.T) {
	t.Parallel()

	val := big.NewInt(999)
	gobData, err := val.GobEncode()
	if err != nil {
		t.Fatal(err)
	}

	rawStore := inmemstore.New()
	if err := rawStore.Put(&rawKeyItem{key: "accounting_balance_peer1", data: gobData}); err != nil {
		t.Fatal(err)
	}

	_, err = storeadapter.NewStateStorerAdapter(rawStore)
	if err == nil {
		t.Fatal("expected migration to fail on Gob-encoded data, got nil error")
	}
}

func TestStateStoreAdapter(t *testing.T) {
	t.Parallel()

	test.Run(t, func(t *testing.T) storage.StateStorer {
		t.Helper()

		store, err := storeadapter.NewStateStorerAdapter(inmemstore.New())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
		})

		return store
	})

	test.RunPersist(t, func(t *testing.T, dir string) storage.StateStorer {
		t.Helper()

		leveldb, err := leveldbstore.New(dir, nil)
		if err != nil {
			t.Fatal(err)
		}

		store, err := storeadapter.NewStateStorerAdapter(leveldb)
		if err != nil {
			t.Fatal(err)
		}

		return store
	})
}
