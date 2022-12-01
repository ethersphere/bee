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
type Repository struct {
	indexStore TxStore
	chunkStore TxChunkStore
}

// IndexStore returns Store.
func (r *Repository) IndexStore() Store {
	return r.indexStore
}

// ChunkStore returns ChunkStore.
func (r *Repository) ChunkStore() ChunkStore {
	return r.chunkStore
}

// NewTx returns a new transaction that guards all the Repository
// stores. The transaction must be committed or rolled back.
func (r *Repository) NewTx(ctx context.Context) (repository *Repository, commit func() error, rollback func() error) {
	tx := NewTxState(ctx)

	repository = new(Repository)
	repository.indexStore = r.indexStore.NewTx(tx)
	repository.chunkStore = r.chunkStore.NewTx(tx)

	txs := []Tx{repository.indexStore, repository.chunkStore}

	commit = func() error {
		for _, tx := range txs {
			if err := tx.Commit(); err != nil {
				return err
			}
		}
		return nil
	}

	rollback = func() error {
		var errs *multierror.Error
		for _, tx := range txs {
			if err := tx.Rollback(); err != nil {
				errs = multierror.Append(errs, err)
			}
		}
		return errs.ErrorOrNil()
	}

	return repository, commit, rollback
}

// NewRepository returns a new Repository instance.
func NewRepository(indexStore TxStore, chunkStore TxChunkStore) *Repository {
	return &Repository{
		indexStore: indexStore,
		chunkStore: chunkStore,
	}
}
