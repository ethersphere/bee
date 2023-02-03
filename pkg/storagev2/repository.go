// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	"context"

	"github.com/hashicorp/go-multierror"
)

// Repository is a collection of stores that provides a unified interface
// to access them. Access to all stores can be guarded by a transaction.
type Repository interface {
	IndexStore() Store
	ChunkStore() ChunkStore

	NewTx(context.Context) (repo Repository, commit func() error, rollback func() error)
}

type repository struct {
	indexStore TxStore
	chunkStore TxChunkStore
}

// IndexStore returns Store.
func (r *repository) IndexStore() Store {
	return r.indexStore
}

// ChunkStore returns ChunkStore.
func (r *repository) ChunkStore() ChunkStore {
	return r.chunkStore
}

// NewTx returns a new transaction that guards all the Repository
// stores. The transaction must be committed or rolled back.
func (r *repository) NewTx(ctx context.Context) (Repository, func() error, func() error) {
	repo := new(repository)
	repo.indexStore = r.indexStore.NewTx(NewTxState(ctx))
	repo.chunkStore = r.chunkStore.NewTx(NewTxState(ctx))

	txs := []Tx{repo.indexStore, repo.chunkStore}

	commit := func() error {
		for _, tx := range txs {
			if err := tx.Commit(); err != nil {
				return err
			}
		}
		return nil
	}

	rollback := func() error {
		var errs *multierror.Error
		for i := len(txs) - 1; i >= 0; i-- {
			if err := txs[i].Rollback(); err != nil {
				errs = multierror.Append(errs, err)
			}
		}
		return errs.ErrorOrNil()
	}

	return repo, commit, rollback
}

// NewRepository returns a new Repository instance.
func NewRepository(indexStore TxStore, chunkStore TxChunkStore) Repository {
	return &repository{
		indexStore: indexStore,
		chunkStore: chunkStore,
	}
}
