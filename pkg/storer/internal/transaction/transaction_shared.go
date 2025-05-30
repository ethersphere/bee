// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Package transaction provides transaction support for localstore operations.
All writes to the localstore (both indexstore and chunkstore) must be made using a transaction.
The transaction must be committed for the writes to be stored on the disk.

The rules of the transaction is as follows:

-sharky_write 		-> write to disk, keep sharky location in memory
-sharky_release		-> keep location in memory, do not release from the disk
-indexstore write	-> write to batch
-on commit			-> if batch_commit succeeds, release sharky_release locations from the disk
					-> if batch_commit fails or is not called, release all sharky_write location from the disk, do nothing for sharky_release

See the NewTransaction method for more details.
*/

package transaction

import (
	"context"
	"errors"

	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type Transaction interface {
	Store
	Commit() error
}

type Store interface {
	ChunkStore() storage.ChunkStore
	IndexStore() storage.IndexStore
}

type ReadOnlyStore interface {
	IndexStore() storage.Reader
	ChunkStore() storage.ReadOnlyChunkStore
}

type Storage interface {
	ReadOnlyStore
	NewTransaction(context.Context) (Transaction, func())
	Run(context.Context, func(Store) error) error
	Close() error
}

// Run creates a new transaction and gives the caller access to the transaction
// in the form of a callback function. After the callback returns, the transaction
// is committed to the disk. See the NewTransaction method for more details on how transactions operate internally.
// By design, it is best to not batch too many writes to a single transaction, including multiple chunks writes.
// Calls made to the transaction are NOT thread-safe.
func (s *store) Run(ctx context.Context, f func(Store) error) error {
	trx, done := s.NewTransaction(ctx)
	defer done()

	err := f(trx)
	if err != nil {
		return err
	}
	return trx.Commit()
}

func (s *store) Close() error {
	return errors.Join(s.bstore.Close(), s.sharky.Close())
}

// IndexStore gives access to the index store of the transaction.
// Note that no writes are persisted to the disk until the commit is called.
func (t *transaction) IndexStore() storage.IndexStore {
	return t.indexstore
}

// ChunkStore gives access to the chunkstore of the transaction.
// Note that no writes are persisted to the disk until the commit is called.
func (t *transaction) ChunkStore() storage.ChunkStore {
	return t.chunkStore
}

func (c *chunkStoreTrx) lock(addr swarm.Address) func() {
	// directly lock
	if c.readOnly {
		c.globalLocker.Lock(addr.ByteString())
		return func() { c.globalLocker.Unlock(addr.ByteString()) }
	}

	// lock chunk only once in the same transaction
	if _, ok := c.lockedAddrs[addr.ByteString()]; !ok {
		c.globalLocker.Lock(addr.ByteString())
		c.lockedAddrs[addr.ByteString()] = struct{}{}
	}

	return func() {} // unlocking the chunk will be done in the Commit()
}

func (s *indexTrx) Get(i storage.Item) error           { return s.store.Get(i) }
func (s *indexTrx) Has(k storage.Key) (bool, error)    { return s.store.Has(k) }
func (s *indexTrx) GetSize(k storage.Key) (int, error) { return s.store.GetSize(k) }

func (s *indexTrx) Count(k storage.Key) (int, error) { return s.store.Count(k) }
func (s *indexTrx) Put(i storage.Item) error         { return s.batch.Put(i) }
func (s *indexTrx) Delete(i storage.Item) error      { return s.batch.Delete(i) }

func (s *sharkyTrx) Release(ctx context.Context, loc sharky.Location) error {
	s.releasedLocs = append(s.releasedLocs, loc)
	return nil
}
