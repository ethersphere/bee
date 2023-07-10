// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"

	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storer/internal/events"
	"github.com/ethersphere/bee/pkg/storer/internal/reserve"
)

var (
	InitShedIndexes = initShedIndexes
	EpochMigration  = epochMigration
)

func (db *DB) Reserve() *reserve.Reserve {
	return db.reserve
}

func (db *DB) Repo() storage.Repository {
	return db.repo
}

func (db *DB) Events() *events.Subscriber {
	return db.events
}

func ReplaceSharkyShardLimit(val int) {
	sharkyNoOfShards = val
}

type wrappedRepo struct {
	storage.Repository
	deleteHook func(storage.Item) error
	putHook    func(storage.Item) error
}

func (w *wrappedRepo) IndexStore() storage.BatchedStore {
	return &wrappedStore{
		BatchedStore: w.Repository.IndexStore(),
		deleteHook:   w.deleteHook,
		putHook:      w.putHook,
	}
}

type wrappedStore struct {
	storage.BatchedStore
	deleteHook func(storage.Item) error
	putHook    func(storage.Item) error
}

func (w *wrappedStore) Put(item storage.Item) error {
	if w.putHook != nil {
		err := w.putHook(item)
		if err != nil {
			return err
		}
	}
	return w.BatchedStore.Put(item)
}

func (w *wrappedStore) Delete(item storage.Item) error {
	if w.deleteHook != nil {
		err := w.deleteHook(item)
		if err != nil {
			return err
		}
	}
	return w.BatchedStore.Delete(item)
}

func (w *wrappedStore) Batch(ctx context.Context) (storage.Batch, error) {
	batch, err := w.BatchedStore.Batch(ctx)
	if err != nil {
		return nil, err
	}
	return &wrappedBatch{
		Batch:      batch,
		deleteHook: w.deleteHook,
		putHook:    w.putHook,
	}, nil
}

type wrappedBatch struct {
	storage.Batch
	deleteHook func(storage.Item) error
	putHook    func(storage.Item) error
}

func (w *wrappedBatch) Put(item storage.Item) error {
	if w.putHook != nil {
		err := w.putHook(item)
		if err != nil {
			return err
		}
	}
	return w.Batch.Put(item)
}

func (w *wrappedBatch) Delete(item storage.Item) error {
	if w.deleteHook != nil {
		err := w.deleteHook(item)
		if err != nil {
			return err
		}
	}
	return w.Batch.Delete(item)
}

func (w *wrappedRepo) NewTx(ctx context.Context) (storage.Repository, func() error, func() error) {
	repo, commit, rollback := w.Repository.NewTx(ctx)
	return &wrappedRepo{
		Repository: repo,
		deleteHook: w.deleteHook,
		putHook:    w.putHook,
	}, commit, rollback
}

func (db *DB) SetRepoStoreDeleteHook(fn func(storage.Item) error) {
	if alreadyWrapped, ok := db.repo.(*wrappedRepo); ok {
		db.repo = &wrappedRepo{Repository: alreadyWrapped.Repository, deleteHook: fn}
		return
	}
	db.repo = &wrappedRepo{Repository: db.repo, deleteHook: fn}
}

func (db *DB) SetRepoStorePutHook(fn func(storage.Item) error) {
	if alreadyWrapped, ok := db.repo.(*wrappedRepo); ok {
		db.repo = &wrappedRepo{Repository: alreadyWrapped.Repository, putHook: fn}
		return
	}
	db.repo = &wrappedRepo{Repository: db.repo, putHook: fn}
}

func (db *DB) WaitForBgCacheWorkers() (unblock func()) {
	for i := 0; i < defaultBgCacheWorkers; i++ {
		db.bgCacheLimiter <- struct{}{}
	}
	return func() {
		for i := 0; i < defaultBgCacheWorkers; i++ {
			<-db.bgCacheLimiter
		}
	}
}

func DefaultOptions() *Options {
	return defaultOptions()
}
