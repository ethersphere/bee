// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"
	"errors"
	"fmt"
	"time"

	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	cacheOverCapacity = "cacheOverCapacity"
)

func (db *DB) cacheWorker(ctx context.Context) {

	defer db.inFlight.Done()

	overCapTrigger, overCapUnsub := db.events.Subscribe(cacheOverCapacity)
	defer overCapUnsub()

	db.triggerCacheEviction()

	for {
		select {
		case <-ctx.Done():
			return
		case <-overCapTrigger:

			size, capc := db.cacheObj.Size(), db.cacheObj.Capacity()
			if size <= capc {
				continue
			}

			evict := size - capc
			if evict < db.reserveOptions.cacheMinEvictCount { // evict at least a min count
				evict = db.reserveOptions.cacheMinEvictCount
			}

			dur := captureDuration(time.Now())
			err := db.cacheObj.RemoveOldest(ctx, db.storage, evict)
			db.metrics.MethodCallsDuration.WithLabelValues("cachestore", "RemoveOldest").Observe(dur())
			if err != nil {
				db.metrics.MethodCalls.WithLabelValues("cachestore", "RemoveOldest", "failure").Inc()
				db.logger.Warning("cache eviction failure", "error", err)
			} else {
				db.logger.Debug("cache eviction finished", "evicted", evict, "duration_sec", dur())
				db.metrics.MethodCalls.WithLabelValues("cachestore", "RemoveOldest", "success").Inc()
			}
			db.triggerCacheEviction()
		case <-db.quit:
			return
		}
	}
}

// Lookup is the implementation of the CacheStore.Lookup method.
func (db *DB) Lookup() storage.Getter {
	return getterWithMetrics{
		storage.GetterFunc(func(ctx context.Context, address swarm.Address) (swarm.Chunk, error) {
			ch, err := db.cacheObj.Getter(db.storage).Get(ctx, address)
			switch {
			case err == nil:
				return ch, nil
			case errors.Is(err, storage.ErrNotFound):
				// here we would ideally have nothing to do but just to return this
				// error to the client. The commit is mainly done to end the txn.
				return nil, err
			}
			// if we are here, it means there was some unexpected error, in which
			// case we need to rollback any changes that were already made.
			return nil, fmt.Errorf("cache.Get: %w", err)
		}),
		db.metrics,
		"cachestore",
	}
}

// Cache is the implementation of the CacheStore.Cache method.
func (db *DB) Cache() storage.Putter {
	return putterWithMetrics{
		storage.PutterFunc(func(ctx context.Context, ch swarm.Chunk) error {
			defer db.triggerCacheEviction()
			err := db.cacheObj.Putter(db.storage).Put(ctx, ch)
			if err != nil {
				return fmt.Errorf("cache.Put: %w", err)
			}
			return nil
		}),
		db.metrics,
		"cachestore",
	}
}

// CacheShallowCopy creates cache entries with the expectation that the chunk already exists in the chunkstore.
func (db *DB) CacheShallowCopy(ctx context.Context, store transaction.Storage, addrs ...swarm.Address) error {
	defer db.triggerCacheEviction()
	dur := captureDuration(time.Now())
	err := db.cacheObj.ShallowCopy(ctx, store, addrs...)
	db.metrics.MethodCallsDuration.WithLabelValues("cachestore", "ShallowCopy").Observe(dur())
	if err != nil {
		err = fmt.Errorf("cache shallow copy: %w", err)
		db.metrics.MethodCalls.WithLabelValues("cachestore", "ShallowCopy", "failure").Inc()
	} else {
		db.metrics.MethodCalls.WithLabelValues("cachestore", "ShallowCopy", "success").Inc()
	}
	return err
}

func (db *DB) triggerCacheEviction() {

	var (
		size = db.cacheObj.Size()
		capc = db.cacheObj.Capacity()
	)
	db.metrics.CacheSize.Set(float64(size))

	if size > capc {
		db.events.Trigger(cacheOverCapacity)
	}
}
