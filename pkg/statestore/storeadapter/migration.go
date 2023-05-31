// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storeadapter

import (
	"strings"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/migration"
)

func AllSteps() migration.Steps {
	return map[uint64]migration.StepFn{
		1: epochMigration,
	}
}

func epochMigration(s storage.Store) error {
	return s.Iterate(storage.Query{
		Factory: func() storage.Item { return &rawItem{&proxyItem{obj: []byte(nil)}} },
	}, func(res storage.Result) (stop bool, err error) {
		if strings.HasPrefix(res.ID, stateStoreNamespace) {
			return false, nil
		}

		item := res.Entry.(*rawItem)
		item.key = res.ID
		item.ns = stateStoreNamespace
		if err := s.Put(item); err != nil {
			return true, err
		}
		return false, nil
	})
}
