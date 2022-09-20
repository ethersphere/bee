// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldbstore

import (
	"context"
	"errors"
	"fmt"
	"sync"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	ldb "github.com/syndtr/goleveldb/leveldb"
)

func (s *Store) Batch(ctx context.Context) (storage.Batch, error) {
	return &Batch{
		ctx:   ctx,
		batch: new(ldb.Batch),
		store: s,
	}, nil
}

type Batch struct {
	mu  sync.Mutex
	ctx context.Context

	batch *ldb.Batch

	store *Store
	done  bool
}

func (i *Batch) Put(item storage.Item) error {
	if i.ctx.Err() != nil {
		return i.ctx.Err()
	}

	key := []byte(item.Namespace() + separator + item.ID())
	value, err := item.Marshal()
	if err != nil {
		return fmt.Errorf("failed serializing: %w", err)
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	i.batch.Put(key, value)

	return nil
}

func (i *Batch) Delete(key storage.Key) error {
	if i.ctx.Err() != nil {
		return i.ctx.Err()
	}

	dbKey := []byte(key.Namespace() + separator + key.ID())

	i.mu.Lock()
	defer i.mu.Unlock()

	i.batch.Delete(dbKey)

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

	if err := i.store.db.Write(i.batch, nil); err != nil {
		return fmt.Errorf("commit batch: %w", err)
	}

	i.done = true

	return nil
}
