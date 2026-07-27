// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storeadapter

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/migration"
)

// allSteps lists all state store migration steps.
// All legacy steps are now NOOPs since all nodes have already run these migrations,
// and new nodes start with an empty database.
func allSteps(st storage.Store) migration.Steps {
	// IMPORTANT: keep this noop and all historical version entries that point to it.
	// migration.Migrate executes steps starting from (storedVersion + 1) and stops
	// at the first missing version. If an old NOOP version is removed/renumbered,
	// nodes that already stored a higher version can start beyond the new steps
	// and never execute newly added migrations.
	noop := func() error { return nil }
	return map[uint64]migration.StepFn{
		1:  noop,
		2:  noop,
		3:  noop,
		4:  noop,
		5:  noop,
		6:  noop,
		7:  noop,
		8:  noop,
		9:  rewriteAddressbookEnvelope(st),
		10: stampAddressbookLastSeen(st),
	}
}

type legacyEntry struct {
	Overlay   string   `json:"overlay"`
	Underlay  string   `json:"underlay,omitempty"`
	Underlays []string `json:"underlays"`
	Signature string   `json:"signature"`
	Nonce     string   `json:"transaction"`
}

type migratedAddress struct {
	Overlay           string   `json:"overlay"`
	Underlays         []string `json:"underlays"`
	Signature         string   `json:"signature"`
	Nonce             string   `json:"nonce"`
	Timestamp         int64    `json:"timestamp"`
	ChequebookAddress string   `json:"chequebook,omitempty"`
}

type migratedEntry struct {
	Address  migratedAddress `json:"address"`
	Verified bool            `json:"verified"`
	LastSeen int64           `json:"last_seen,omitempty"`
}

// rewriteAddressbookEnvelope wraps each "addressbook_entry_*" legacy
// bzz.Address payload as {address, verified:false}. Undecodable records
// are dropped; already-migrated entries pass through. Two-pass (collect,
// then write) avoids mutating under the iterator. The legacy shape tagged
// the nonce "transaction"; current code uses "nonce".
func rewriteAddressbookEnvelope(s storage.Store) migration.StepFn {
	return func() error {
		store := &StateStorerAdapter{s}

		type item struct {
			key string
			val []byte
		}

		var batch []item
		if err := store.Iterate("addressbook_entry_", func(key, val []byte) (stop bool, err error) {
			batch = append(batch, item{
				key: string(key),
				val: append([]byte(nil), val...),
			})
			return false, nil
		}); err != nil {
			return fmt.Errorf("iterate addressbook entries: %w", err)
		}

		for _, e := range batch {
			var probe map[string]json.RawMessage
			if json.Unmarshal(e.val, &probe) == nil {
				if _, ok := probe["address"]; ok {
					continue
				}
			}

			var legacy legacyEntry
			if err := json.Unmarshal(e.val, &legacy); err != nil || legacy.Overlay == "" {
				_ = store.Delete(e.key)
				continue
			}

			underlays := legacy.Underlays
			if len(underlays) == 0 && legacy.Underlay != "" {
				underlays = []string{legacy.Underlay}
			}

			out := migratedEntry{
				Address: migratedAddress{
					Overlay:   legacy.Overlay,
					Underlays: underlays,
					Signature: legacy.Signature,
					Nonce:     legacy.Nonce,
				},
			}

			if err := store.Put(e.key, &out); err != nil {
				return fmt.Errorf("rewrite addressbook entry %q: %w", e.key, err)
			}
		}

		return nil
	}
}

// stampAddressbookLastSeen sets "last_seen" to the current time on every
// "addressbook_entry_*" record that lacks it, so that addresses carried over
// from before pruning was introduced are not immediately pruned. Entries that
// already carry a non-zero last_seen are left untouched. The record is decoded
// into migratedEntry, the current serialization shape, whose last_seen field is
// omitempty so older records that predate it round-trip unchanged.
func stampAddressbookLastSeen(s storage.Store) migration.StepFn {
	return func() error {
		store := &StateStorerAdapter{s}

		type item struct {
			key string
			val []byte
		}

		var batch []item
		if err := store.Iterate("addressbook_entry_", func(key, val []byte) (stop bool, err error) {
			batch = append(batch, item{
				key: string(key),
				val: append([]byte(nil), val...),
			})
			return false, nil
		}); err != nil {
			return fmt.Errorf("iterate addressbook entries: %w", err)
		}

		now := time.Now().Unix()

		for _, e := range batch {
			var entry migratedEntry
			if err := json.Unmarshal(e.val, &entry); err != nil {
				_ = store.Delete(e.key)
				continue
			}

			if entry.LastSeen != 0 {
				continue
			}
			entry.LastSeen = now

			if err := store.Put(e.key, &entry); err != nil {
				return fmt.Errorf("stamp addressbook entry %q: %w", e.key, err)
			}
		}

		return nil
	}
}
