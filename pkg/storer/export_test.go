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

func (w *wrappedRepo) IndexStore() storage.Store {
	return &wrappedStore{
		Store:      w.Repository.IndexStore(),
		deleteHook: w.deleteHook,
		putHook:    w.putHook,
	}
}

type wrappedStore struct {
	storage.Store
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
	return w.Store.Put(item)
}

func (w *wrappedStore) Delete(item storage.Item) error {
	if w.deleteHook != nil {
		err := w.deleteHook(item)
		if err != nil {
			return err
		}
	}
	return w.Store.Delete(item)
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
		db.bgCacheWorkers <- struct{}{}
	}
	return func() {
		for i := 0; i < defaultBgCacheWorkers; i++ {
			<-db.bgCacheWorkers
		}
	}
}

func DefaultOptions() *Options {
	return defaultOptions()
}
