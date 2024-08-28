// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storeadapter

import (
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

		var deleteEntries = []string{
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
