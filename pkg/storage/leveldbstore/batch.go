// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldbstore

import (
	"context"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/storage"
	ldb "github.com/syndtr/goleveldb/leveldb"
)

// Batch implements storage.BatchedStore interface Batch method.
func (s *Store) Batch(ctx context.Context) storage.Batch {
	return &Batch{
		ctx:   ctx,
		batch: new(ldb.Batch),
		store: s,
	}
}

type Batch struct {
	ctx context.Context

	mu    sync.Mutex // mu guards batch and done.
	batch *ldb.Batch
	store *Store
	done  bool
}

// Put implements storage.Batch interface Put method.
func (i *Batch) Put(item storage.Item) error {
	if err := i.ctx.Err(); err != nil {
		return err
	}

	val, err := item.Marshal()
	if err != nil {
		return fmt.Errorf("unable to marshal item: %w", err)
	}

	i.mu.Lock()
	i.batch.Put(key(item), val)
	i.mu.Unlock()

	return nil
}

// Delete implements storage.Batch interface Delete method.
func (i *Batch) Delete(item storage.Item) error {
	if err := i.ctx.Err(); err != nil {
		return err
	}

	i.mu.Lock()
	i.batch.Delete(key(item))
	i.mu.Unlock()

	return nil
}

// Commit implements storage.Batch interface Commit method.
func (i *Batch) Commit() error {
	if err := i.ctx.Err(); err != nil {
		return err
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	if i.done {
		return storage.ErrBatchCommitted
	}

	if err := i.store.db.Write(i.batch, nil); err != nil {
		return fmt.Errorf("unable to commit batch: %w", err)
	}

	i.done = true

	return nil
}
