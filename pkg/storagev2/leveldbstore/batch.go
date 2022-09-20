// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldbstore

import (
	"context"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	ldb "github.com/syndtr/goleveldb/leveldb"
)

type lvldbCommiter struct {
	store *Store
}

func (c *lvldbCommiter) Commit(ops map[string]storage.BatchOp) error {
	batch := new(ldb.Batch)

	for range ops {
		// TODO construct batch from ops
	}

	return c.store.db.Write(batch, nil)
}

func (s *Store) Batch(ctx context.Context) (storage.Batch, error) {
	return storage.NewOpBatcher(ctx, &lvldbCommiter{store: s}), nil
}
