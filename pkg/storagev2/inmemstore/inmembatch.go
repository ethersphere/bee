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

func (s *Store) Batch(ctx context.Context) (storage.Batch, error) {
	return &Batch{
		mu:    new(sync.Mutex),
		ctx:   ctx,
		store: s,
	}, nil
}

type Batch struct {
	mu     *sync.Mutex
	ctx    context.Context
	put    []storage.Item
	delete []storage.Key
	store  *Store
	done   bool
}

func (i *Batch) Put(item storage.Item) error {
	if i.ctx.Err() != nil {
		return i.ctx.Err()
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	for _, put := range i.put {
		if put.ID() == item.ID() {
			return nil // disallow duplicates
		}
	}

	i.put = append(i.put, item)

	return nil
}

func (i *Batch) Delete(key storage.Key) error {
	if i.ctx.Err() != nil {
		return i.ctx.Err()
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	for index, put := range i.put {
		if put.ID() == key.ID() {
			i.put = append(i.put[:index], i.put[index+1:]...)
			return nil
		}
	}

	for _, del := range i.delete {
		if del.ID() == key.ID() {
			return nil // disallow duplicates
		}
	}

	i.delete = append(i.delete, key)

	return nil
}

func (i *Batch) Commit() error {
	if i.ctx.Err() != nil {
		return i.ctx.Err()
	}

	i.store.mu.Lock()
	defer i.store.mu.Unlock()

	if i.done {
		return errors.New("already committed")
	}

	for _, item := range i.put {
		if err := i.store.put(item); err != nil {
			return err
		}
	}

	for _, item := range i.delete {
		if err := i.store.delete(item); err != nil {
			return err
		}
	}

	i.done = true

	return nil
}
