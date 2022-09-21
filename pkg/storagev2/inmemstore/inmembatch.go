// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemstore

import (
	"context"
	"errors"
	"sync"

	storage "github.com/ethersphere/bee/pkg/storagev2"
)

type batchOp struct {
	delete bool
	item   storage.Item
}

type Batch struct {
	mu    sync.Mutex
	ctx   context.Context
	ops   map[string]batchOp
	store *Store
	done  bool
}

func (s *Store) Batch(ctx context.Context) (storage.Batch, error) {
	return &Batch{
		ctx:   ctx,
		ops:   make(map[string]batchOp),
		store: s,
	}, nil
}

func (i *Batch) Put(item storage.Item) error {
	if i.ctx.Err() != nil {
		return i.ctx.Err()
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	key := getKeyString(item)

	i.ops[key] = batchOp{
		item: item,
	}

	return nil
}

func (i *Batch) Delete(key storage.Key) error {
	if i.ctx.Err() != nil {
		return i.ctx.Err()
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	sKey := getKeyString(key)

	i.ops[sKey] = batchOp{
		delete: true,
	}

	return nil
}

func (i *Batch) Commit() error {
	if i.ctx.Err() != nil {
		return i.ctx.Err()
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	if i.done {
		return errors.New("already committed")
	}

	defer func() { i.done = true }()

	i.store.mu.Lock()
	defer i.store.mu.Unlock()

	for key, ops := range i.ops {
		if ops.delete {
			if err := i.store.delete(key); err != nil {
				return err
			}
			continue
		}
		if err := i.store.put(ops.item); err != nil {
			return err
		}
	}

	return nil
}
