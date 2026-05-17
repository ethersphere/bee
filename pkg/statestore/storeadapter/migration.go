// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storeadapter

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethersphere/bee/v2/pkg/bigint"
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
		9: migrateBigIntKeys(st),
	}
}

// bigIntPrefixes lists all statestore key prefixes whose values are stored as
// big.Int and need to be migrated to bigint.BigInt binary (Gob) encoding.
var bigIntPrefixes = []string{
	"accounting_balance_",
	"accounting_surplusbalance_",
	"accounting_originatedbalance_",
	"swap_chequebook_total_issued_",
}

// migrateBigIntKeys rewrites all balance values from their legacy JSON encoding
// (unquoted decimal from json.Marshal(*big.Int), or quoted string from
// json.Marshal(bigint.BigInt)) to the canonical Gob binary encoding produced
// by bigint.BigInt.MarshalBinary. After this migration UnmarshalBinary only
// needs to handle Gob.
func migrateBigIntKeys(s storage.Store) migration.StepFn {
	return func() error {
		store := &StateStorerAdapter{s}
		for _, prefix := range bigIntPrefixes {
			var rewrite []struct {
				key string
				val *big.Int
			}

			err := store.Iterate(prefix, func(key, data []byte) (bool, error) {
				if len(data) == 0 {
					return false, nil
				}

				v := new(big.Int)
				switch data[0] {
				case 2, 3:
					// Gob-encoded data found before migration — database is in an unexpected state
					return true, fmt.Errorf("unexpected Gob-encoded bigint at key %q before migration", key)
				case '"':
					// quoted decimal string from json.Marshal(bigint.BigInt)
					var w bigint.BigInt
					if err := json.Unmarshal(data, &w); err != nil {
						return true, fmt.Errorf("unmarshal quoted bigint at key %q: %w", key, err)
					}
					v = w.Int
				default:
					// unquoted decimal from json.Marshal(*big.Int)
					if _, ok := v.SetString(string(data), 10); !ok {
						return true, fmt.Errorf("parse decimal bigint at key %q: invalid value %q", key, data)
					}
				}

				// The StateStorerAdapter iterator gives key = prefix + res.ID,
				// where res.ID already contains the full key (including prefix).
				// Strip the leading prefix duplicate to get the actual statestore key.
				rewrite = append(rewrite, struct {
					key string
					val *big.Int
				}{string(key)[len(prefix):], v})
				return false, nil
			})
			if err != nil {
				return err
			}

			for _, r := range rewrite {
				if err := store.Put(r.key, &bigint.BigInt{Int: r.val}); err != nil {
					return fmt.Errorf("rewrite bigint key %q: %w", r.key, err)
				}
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
