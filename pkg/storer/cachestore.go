// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"errors"
	"fmt"

	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	cacheAccessLockKey = "cachestoreAccess"
)

// Lookup is the implementation of the CacheStore.Lookup method.
func (db *DB) Lookup() storage.Getter {
	return getterWithMetrics{
		storage.GetterFunc(func(ctx context.Context, address swarm.Address) (swarm.Chunk, error) {
			// the cacheObj resets its state on failures and expects the transaction
			// rollback to undo all the updates, so we need a lock here to prevent
			// concurrent access to the cacheObj.
			db.lock.Lock(cacheAccessLockKey)
			defer db.lock.Unlock(cacheAccessLockKey)

			txnRepo, commit, rollback := db.repo.NewTx(ctx)
			ch, err := db.cacheObj.Getter(txnRepo).Get(ctx, address)
			switch {
			case err == nil:
				return ch, commit()
			case errors.Is(err, storage.ErrNotFound):
				// here we would ideally have nothing to do but just to return this
				// error to the client. The commit is mainly done to end the txn.
				return nil, errors.Join(err, commit())
			}
			// if we are here, it means there was some unexpected error, in which
			// case we need to rollback any changes that were already made.
			return nil, fmt.Errorf("cache.Get: %w", errors.Join(err, rollback()))
		}),
		db.metrics,
		"cachestore",
	}
}

// Cache is the implementation of the CacheStore.Cache method.
func (db *DB) Cache() storage.Putter {
	return putterWithMetrics{
		storage.PutterFunc(func(ctx context.Context, ch swarm.Chunk) error {
			// the cacheObj resets its state on failures and expects the transaction
			// rollback to undo all the updates, so we need a lock here to prevent
			// concurrent access to the cacheObj.
			db.lock.Lock(cacheAccessLockKey)
			defer db.lock.Unlock(cacheAccessLockKey)

			txnRepo, commit, rollback := db.repo.NewTx(ctx)
			err := db.cacheObj.Putter(txnRepo).Put(ctx, ch)
			if err != nil {
				return fmt.Errorf("cache.Put: %w", errors.Join(err, rollback()))
			}
			db.metrics.CacheSize.Set(float64(db.cacheObj.Size()))
			return errors.Join(err, commit())
		}),
		db.metrics,
		"cachestore",
	}
}
