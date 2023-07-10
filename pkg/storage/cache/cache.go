// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	"context"
	"errors"
	"sync"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/storageutil"
	"github.com/hashicorp/golang-lru/v2"
)

var _ storage.BatchedStore = (*Cache)(nil)

// key returns a string representation of the given key.
func key(key storage.Key) string {
	return storageutil.JoinFields(key.Namespace(), key.ID())
}

// Cache is a wrapper around a storage.BatchedStore that adds
// a layer of in-memory caching for the Get and Has operations.
type Cache struct {
	storage.BatchedStore

	lru     *lru.Cache[string, []byte]
	syncMu  *sync.RWMutex
	metrics metrics
}

// Wrap adds a layer of in-memory caching to basic BatchedStore operations.
func Wrap(store storage.BatchedStore, capacity int) (*Cache, error) {
	if _, ok := store.(storage.Tx); ok {
		return nil, errors.New("cache should not be used with transactions")
	}

	lru, err := lru.New[string, []byte](capacity)
	if err != nil {
		return nil, err
	}

	return &Cache{store, lru, new(sync.RWMutex), newMetrics()}, nil
}

// MustWrap is like Wrap but panics if the capacity is less than
// or equal to zero or if the given store implements storage.Tx.
func MustWrap(store storage.BatchedStore, capacity int) *Cache {
	c, err := Wrap(store, capacity)
	if err != nil {
		panic(err)
	}
	return c
}

// add caches given item.
func (c *Cache) add(i storage.Item) {
	if b, err := i.Marshal(); err == nil {
		c.lru.Add(key(i), b)
	}
}

// Get implements storage.BatchedStore interface.
// On a call it tries to first retrieve the item from cache.
// If the item does not exist in cache, it tries to retrieve
// it from the underlying store.
func (c *Cache) Get(i storage.Item) error {
	c.syncMu.RLock()
	defer c.syncMu.RUnlock()

	if val, ok := c.lru.Get(key(i)); ok {
		c.metrics.CacheHit.Inc()
		return i.Unmarshal(val)
	}

	if err := c.BatchedStore.Get(i); err != nil {
		return err
	}

	c.metrics.CacheMiss.Inc()
	c.add(i)

	return nil
}

// Has implements storage.BatchedStore interface.
// On a call it tries to first retrieve the item from cache.
// If the item does not exist in cache, it tries to retrieve
// it from the underlying store.
func (c *Cache) Has(k storage.Key) (bool, error) {
	c.syncMu.RLock()
	defer c.syncMu.RUnlock()

	if _, ok := c.lru.Get(key(k)); ok {
		c.metrics.CacheHit.Inc()
		return true, nil
	}

	c.metrics.CacheMiss.Inc()
	return c.BatchedStore.Has(k)
}

// Put implements storage.BatchedStore interface.
// On a call it also inserts the item into the cache so that the next
// call to Put and Has will be able to retrieve the item from cache.
func (c *Cache) Put(i storage.Item) error {
	c.syncMu.RLock()
	defer c.syncMu.RUnlock()

	c.add(i)
	return c.BatchedStore.Put(i)
}

// Delete implements storage.BatchedStore interface.
// On a call it also removes the item from the cache.
func (c *Cache) Delete(i storage.Item) error {
	c.syncMu.RLock()
	defer c.syncMu.RUnlock()

	_ = c.lru.Remove(key(i))
	return c.BatchedStore.Delete(i)
}

// Batch implements storage.BatchedStore interface.
func (c *Cache) Batch(ctx context.Context) (storage.Batch, error) {
	batch, err := c.BatchedStore.Batch(ctx)
	if err != nil {
		return nil, err
	}
	return &batchOpsTracker{
		batch,
		new(sync.Map),
		c.syncMu,
		func(key string) { c.lru.Remove(key) },
	}, nil
}

// batchOpsTracker is a wrapper around storage.Batch that also
// keeps track of the keys that are added or deleted.
// When the batchOpsTracker is committed, we need to invalidate
// the cache for those keys added or deleted.
type batchOpsTracker struct {
	storage.Batch

	keys       *sync.Map
	syncMu     *sync.RWMutex
	invalidate func(key string)
}

// Put implements storage.Batch interface.
// On a call it also keeps track of the key that is added.
func (b *batchOpsTracker) Put(i storage.Item) error {
	b.keys.Store(i.ID(), struct{}{})
	return b.Batch.Put(i)
}

// Delete implements storage.Batch interface.
// On a call it also keeps track of the key that is deleted.
func (b *batchOpsTracker) Delete(i storage.Item) error {
	b.keys.Store(i.ID(), struct{}{})
	return b.Batch.Delete(i)
}

// Commit implements storage.Batch interface.
// On a call it also invalidates the cache
// for the keys that were added or deleted.
func (b *batchOpsTracker) Commit() error {
	b.syncMu.Lock()
	defer b.syncMu.Unlock()

	if err := b.Batch.Commit(); err != nil {
		return err
	}

	b.keys.Range(func(key, _ interface{}) bool {
		b.invalidate(key.(string))
		return true
	})
	b.keys = new(sync.Map)
	return nil
}
