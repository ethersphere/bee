// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"context"
	"errors"
	"strings"
	"sync"
)

type Batch interface {
	// We will only need to batch write operations
	Put(Item) error
	Delete(Key) error

	Commit() error
}

type BatchedStore interface {
	Store
	Batch(context.Context) (Batch, error)
}

type Commiter interface {
	Commit(ops map[string]BatchOp) error
}

type BatchOp struct {
	Delete bool
	Item   Item
}

type batchImpl struct {
	mu   sync.Mutex
	ctx  context.Context
	ops  map[string]BatchOp
	done bool
	c    Commiter
}

func NewOpBatcher(ctx context.Context, c Commiter) Batch {
	return &batchImpl{
		ctx: ctx,
		ops: make(map[string]BatchOp),
		c:   c,
	}
}

func (i *batchImpl) Put(item Item) error {
	if i.ctx.Err() != nil {
		return i.ctx.Err()
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	key := getKeyString(item)

	i.ops[key] = BatchOp{
		Item: item,
	}

	return nil
}

func (i *batchImpl) Delete(key Key) error {
	if i.ctx.Err() != nil {
		return i.ctx.Err()
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	sKey := getKeyString(key)

	i.ops[sKey] = BatchOp{
		Delete: true,
	}

	return nil
}

func (i *batchImpl) Commit() error {
	if i.ctx.Err() != nil {
		return i.ctx.Err()
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	if i.done {
		return errors.New("already committed")
	}

	err := i.c.Commit(i.ops)

	i.done = true

	return err
}

func getKeyString(i Key) string {
	return strings.Join([]string{i.Namespace(), i.ID()}, "-")
}
