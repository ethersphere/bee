// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemstore

import (
	"context"
	"fmt"
	"sync"

	storage "github.com/ethersphere/bee/v2/pkg/storage"
)

// batchOp represents a batch operations.
type batchOp interface {
	Item() storage.Item
}

// batchOpBase is a base type for batch operations holding data.
type batchOpBase struct{ item storage.Item }

// Item implements batchOp interface Item method.
func (b batchOpBase) Item() storage.Item { return b.item }

type (
	batchOpPut    struct{ batchOpBase }
	batchOpDelete struct{ batchOpBase }
)

type Batch struct {
	ctx context.Context

	mu    sync.Mutex // mu guards batch, ops, and done.
	ops   map[string]batchOp
	store *Store
	done  bool
}

// Batch implements storage.BatchedStore interface Batch method.
func (s *Store) Batch(ctx context.Context) storage.Batch {
	return &Batch{
		ctx:   ctx,
		ops:   make(map[string]batchOp),
		store: s,
	}
}

// Put implements storage.Batch interface Put method.
func (i *Batch) Put(item storage.Item) error {
	if err := i.ctx.Err(); err != nil {
		return err
	}

	i.mu.Lock()
	i.ops[key(item)] = batchOpPut{batchOpBase{item: item}}
	i.mu.Unlock()

	return nil
}

// Delete implements storage.Batch interface Delete method.
func (i *Batch) Delete(item storage.Item) error {
	if err := i.ctx.Err(); err != nil {
		return err
	}

	i.mu.Lock()
	i.ops[key(item)] = batchOpDelete{batchOpBase{item: item}}
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

	defer func() { i.done = true }()

	i.store.mu.Lock()
	defer i.store.mu.Unlock()

	for _, ops := range i.ops {
		switch op := ops.(type) {
		case batchOpPut:
			err := i.store.put(op.Item())
			if err != nil {
				return fmt.Errorf("unable to put item %s: %w", key(op.Item()), err)
			}
		case batchOpDelete:
			err := i.store.delete(op.Item())
			if err != nil {
				return fmt.Errorf("unable to delete item %s: %w", key(op.Item()), err)
			}

		}
	}

	return nil
}
