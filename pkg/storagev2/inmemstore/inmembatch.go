// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemstore

import (
	"context"

	storage "github.com/ethersphere/bee/pkg/storagev2"
)

func (s *Store) Batch(ctx context.Context) (storage.Batch, error) {
	return storage.NewOpBatcher(ctx, &inmemCommiter{store: s}), nil
}

type inmemCommiter struct {
	store *Store
}

func (c *inmemCommiter) Commit(ops map[string]storage.BatchOp) error {
	c.store.mu.Lock()
	defer c.store.mu.Unlock()

	for key, ops := range ops {
		if ops.Delete {
			if err := c.store.delete(key); err != nil {
				return err
			}
			continue
		}
		if err := c.store.put(ops.Item); err != nil {
			return err
		}
	}
	return nil
}
