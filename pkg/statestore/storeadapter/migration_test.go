// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storeadapter_test

import (
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/addressbook"
	"github.com/ethersphere/bee/v2/pkg/statestore/storeadapter"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// Uses leveldb rather than inmemstore: StateStorerAdapter.Iterate
// reconstructs keys for leveldb's prefix-stripping semantics, while
// inmemstore strips only the namespace and would produce double-prefixed
// keys here.
func newTestStore(t *testing.T) storage.Store {
	t.Helper()
	leveldb, _, err := leveldbstore.New(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("leveldbstore.New: %v", err)
	}
	t.Cleanup(func() { _ = leveldb.Close() })
	return leveldb
}

func TestRewriteAddressbookEnvelope(t *testing.T) {
	t.Parallel()

	raw := newTestStore(t)
	store, err := storeadapter.NewStateStorerAdapter(raw)
	if err != nil {
		t.Fatalf("NewStateStorerAdapter: %v", err)
	}

	const prefix = "addressbook_entry_"

	legacy := storeadapter.LegacyEntry{
		Overlay:   "aabb",
		Underlays: []string{"/ip4/1.1.1.1"},
		Signature: "sig==",
		Nonce:     "deadbeef",
	}
	legacyKey := prefix + "aabb"
	if err := store.Put(legacyKey, &legacy); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}

	legacySingle := storeadapter.LegacyEntry{
		Overlay:   "ccdd",
		Underlay:  "/ip4/2.2.2.2",
		Signature: "sig==",
		Nonce:     "deadbeef",
	}
	singleKey := prefix + "ccdd"
	if err := store.Put(singleKey, &legacySingle); err != nil {
		t.Fatalf("seed single: %v", err)
	}

	migrated := storeadapter.MigratedEntry{
		Address: storeadapter.MigratedAddress{
			Overlay:   "eeff",
			Underlays: []string{"/ip4/3.3.3.3"},
			Signature: "newsig==",
			Nonce:     "deadbeef",
			Timestamp: 12345,
		},
		Verified: true,
	}
	migratedKey := prefix + "eeff"
	if err := store.Put(migratedKey, &migrated); err != nil {
		t.Fatalf("seed migrated: %v", err)
	}

	emptyKey := prefix + "0000"
	if err := store.Put(emptyKey, &storeadapter.LegacyEntry{Signature: "s"}); err != nil {
		t.Fatalf("seed empty: %v", err)
	}

	if err := storeadapter.RewriteAddressbookEnvelope(raw)(); err != nil {
		t.Fatalf("migration: %v", err)
	}

	var got storeadapter.MigratedEntry
	if err := store.Get(legacyKey, &got); err != nil {
		t.Fatalf("get legacy after: %v", err)
	}
	if got.Verified {
		t.Fatal("legacy entry: Verified must be false")
	}
	if got.Address.Timestamp != 0 {
		t.Fatalf("legacy entry: Timestamp must be 0, got %d", got.Address.Timestamp)
	}
	if got.Address.Overlay != "aabb" {
		t.Fatalf("legacy entry: Overlay=%q want aabb", got.Address.Overlay)
	}
	if len(got.Address.Underlays) != 1 || got.Address.Underlays[0] != "/ip4/1.1.1.1" {
		t.Fatalf("legacy entry: Underlays=%v", got.Address.Underlays)
	}
	if got.Address.Nonce != "deadbeef" {
		t.Fatalf("legacy entry: Nonce=%q", got.Address.Nonce)
	}

	var gotSingle storeadapter.MigratedEntry
	if err := store.Get(singleKey, &gotSingle); err != nil {
		t.Fatalf("get single after: %v", err)
	}
	if len(gotSingle.Address.Underlays) != 1 || gotSingle.Address.Underlays[0] != "/ip4/2.2.2.2" {
		t.Fatalf("single underlay: got %v", gotSingle.Address.Underlays)
	}

	var gotMig storeadapter.MigratedEntry
	if err := store.Get(migratedKey, &gotMig); err != nil {
		t.Fatalf("get migrated after: %v", err)
	}
	if !gotMig.Verified {
		t.Fatal("already-migrated entry: Verified flipped to false")
	}
	if gotMig.Address.Timestamp != 12345 {
		t.Fatalf("already-migrated entry: Timestamp lost, got %d", gotMig.Address.Timestamp)
	}

	var sink storeadapter.MigratedEntry
	if err := store.Get(emptyKey, &sink); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("empty-overlay entry should have been deleted, got err=%v", err)
	}
}

// Catches the case where the wire-format unit tests pass but
// bzz.Address.UnmarshalJSON rejects the migrated payload at runtime.
func TestRewriteAddressbookEnvelope_AddressbookConsumes(t *testing.T) {
	t.Parallel()

	raw := newTestStore(t)
	store, err := storeadapter.NewStateStorerAdapter(raw)
	if err != nil {
		t.Fatalf("NewStateStorerAdapter: %v", err)
	}

	overlay := swarm.MustParseHexAddress("aabb")
	const (
		validSignature = "c2lnbmF0dXJl" // base64("signature")
		validUnderlay  = "/ip4/127.0.0.1/tcp/1634"
		nonceHex       = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	)

	if err := store.Put("addressbook_entry_"+overlay.String(), &storeadapter.LegacyEntry{
		Overlay:   overlay.String(),
		Underlays: []string{validUnderlay},
		Signature: validSignature,
		Nonce:     nonceHex,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := storeadapter.RewriteAddressbookEnvelope(raw)(); err != nil {
		t.Fatalf("migration: %v", err)
	}

	book := addressbook.New(store)
	got, verified, err := book.Get(overlay)
	if err != nil {
		t.Fatalf("addressbook.Get after migration: %v", err)
	}
	if verified {
		t.Fatal("migrated record must be unverified")
	}
	if !got.Overlay.Equal(overlay) {
		t.Fatalf("overlay mismatch: got %s want %s", got.Overlay, overlay)
	}
	if len(got.Underlays) != 1 || got.Underlays[0].String() != validUnderlay {
		t.Fatalf("underlays: got %v want [%s]", got.Underlays, validUnderlay)
	}
	if got.Timestamp != 0 {
		t.Fatalf("timestamp: got %d want 0", got.Timestamp)
	}
	if got.ChequebookAddress != (common.Address{}) {
		t.Fatalf("chequebook: got %s want zero", got.ChequebookAddress.Hex())
	}
	if len(got.Nonce) == 0 {
		t.Fatal("nonce dropped during migration")
	}
	if len(got.Signature) == 0 {
		t.Fatal("signature dropped during migration")
	}
}

func TestRewriteAddressbookEnvelope_Idempotent(t *testing.T) {
	t.Parallel()

	raw := newTestStore(t)
	store, err := storeadapter.NewStateStorerAdapter(raw)
	if err != nil {
		t.Fatalf("NewStateStorerAdapter: %v", err)
	}

	key := "addressbook_entry_aabb"
	if err := store.Put(key, &storeadapter.LegacyEntry{
		Overlay:   "aabb",
		Underlays: []string{"/ip4/1.1.1.1"},
		Signature: "sig==",
		Nonce:     "deadbeef",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := storeadapter.RewriteAddressbookEnvelope(raw)(); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}

	var got storeadapter.MigratedEntry
	if err := store.Get(key, &got); err != nil {
		t.Fatalf("get after repeated runs: %v", err)
	}
	if got.Verified || got.Address.Timestamp != 0 || got.Address.Overlay != "aabb" {
		t.Fatalf("entry mutated by repeated runs: %+v", got)
	}
}
