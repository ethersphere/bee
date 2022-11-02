// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemchunkstore

import (
	"context"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/hashicorp/go-multierror"
)

// opCode represents code for Store operations.
type opCode string

const (
	putOp    opCode = "put"
	deleteOp opCode = "delete"
)

// op represents an operation that can be invoked by calling the fn.
type op struct {
	// origin is the opCode of the operation that
	// is the originator of this inverse operation.
	origin opCode

	// addr is the address of a chunk on which
	// the inverse operation is performed.
	addr swarm.Address

	// fn is the inverse operation to the origin.
	fn func() error
}

var _ storage.TxChunkStore = (*TxChunkStore)(nil)

// TxChunkStore is an implementation of in-memory Store
// where all Store operations are done in a transaction.
type TxChunkStore struct {
	*storage.TxChunkStoreBase

	// Bookkeeping of invasive operations executed
	// on the ChunkStore to support rollback functionality.
	opsMu sync.Mutex
	ops   []op
}

// Put implements the Store interface.
func (s *TxChunkStore) Put(ctx context.Context, chunk swarm.Chunk) (exists bool, err error) {
	exists, err = s.TxChunkStoreBase.Put(ctx, chunk)
	if err == nil {
		s.opsMu.Lock()
		s.ops = append(s.ops, op{putOp, chunk.Address(), func() error {
			return s.TxChunkStoreBase.ChunkStore.Delete(ctx, chunk.Address())
		}})
		s.opsMu.Unlock()
	}
	return exists, err
}

// Delete implements the Store interface.
func (s *TxChunkStore) Delete(ctx context.Context, addr swarm.Address) error {
	chunk, err := s.Get(ctx, addr)
	if err != nil {
		return err
	}
	err = s.TxChunkStoreBase.Delete(ctx, addr)
	if err == nil {
		s.opsMu.Lock()
		s.ops = append(s.ops, op{deleteOp, addr, func() error {
			_, err := s.TxChunkStoreBase.ChunkStore.Put(ctx, chunk)
			return err
		}})
		s.opsMu.Unlock()
	}
	return err
}

// Commit implements the Tx interface.
func (s *TxChunkStore) Commit() error {
	if err := s.IsDone(); err != nil {
		return err
	}
	s.TxState.Done()
	return nil
}

// Rollback implements the Tx interface.
func (s *TxChunkStore) Rollback() error {
	if err := s.IsDone(); err != nil {
		return err
	}
	var opErrors *multierror.Error
	for _, op := range s.ops {
		if err := op.fn(); err != nil {
			err = fmt.Errorf(
				"inmemchunkstore: unable to rollback operation %q for chunk %s: %w",
				op.origin,
				op.addr,
				err,
			)
			opErrors = multierror.Append(opErrors, err)
		}
	}
	s.TxState.Done()
	return opErrors.ErrorOrNil()
}

// NewTxChunkStore returns an implementation of in-memory ChunkStore
// where all Store operations are done in a transaction.
func NewTxChunkStore(base *storage.TxChunkStoreBase) *TxChunkStore {
	return &TxChunkStore{TxChunkStoreBase: base}
}
