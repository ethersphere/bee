// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storeadapter

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ethersphere/bee/v2/pkg/puller"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/migration"
)

func allSteps(st storage.Store) migration.Steps {
	return map[uint64]migration.StepFn{
		1: epochMigration(st),
		2: deletePrefix(st, puller.IntervalPrefix),
		3: deletePrefix(st, puller.IntervalPrefix),
		4: deletePrefix(st, "blocklist"),
		5: deletePrefix(st, "batchstore"),
		6: deletePrefix(st, puller.IntervalPrefix),
		7: deletePrefix(st, puller.IntervalPrefix),
		8: deletePrefix(st, puller.IntervalPrefix),
		9: rewriteAddressbookEnvelope(st),
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

func deletePrefix(s storage.Store, prefix string) migration.StepFn {
	return func() error {
		store := &StateStorerAdapter{s}
		return store.Iterate(prefix, func(key, val []byte) (stop bool, err error) {
			return false, store.Delete(string(key))
		})
	}
}

func epochMigration(s storage.Store) migration.StepFn {
	return func() error {
		deleteEntries := []string{
			"statestore_schema",
			"tags",
			puller.IntervalPrefix,
			"kademlia-counters",
			"addressbook",
			"batch",
		}

		return s.Iterate(storage.Query{
			Factory: func() storage.Item { return &rawItem{&proxyItem{obj: []byte(nil)}} },
		}, func(res storage.Result) (stop bool, err error) {
			if strings.HasPrefix(res.ID, stateStoreNamespace) {
				return false, nil
			}
			for _, e := range deleteEntries {
				if strings.HasPrefix(res.ID, e) {
					_ = s.Delete(&rawItem{&proxyItem{key: res.ID}})
					return false, nil
				}
			}

			item := res.Entry.(*rawItem)
			item.key = res.ID
			item.ns = stateStoreNamespace
			if err := s.Put(item); err != nil {
				return true, err
			}
			_ = s.Delete(&rawItem{&proxyItem{key: res.ID}})
			return false, nil
		})
	}
}
