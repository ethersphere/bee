// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/storageutil"
	lru "github.com/hashicorp/golang-lru/v2"
)

// key returns a string representation of the given key.
func key(key storage.Key) string {
	return storageutil.JoinFields(key.Namespace(), key.ID())
}

var _ storage.IndexStore = (*Cache)(nil)

// Cache is a wrapper around a storage.Store that adds a layer
// of in-memory caching for the Get and Has operations.
type Cache struct {
	storage.IndexStore

	lru     *lru.Cache[string, []byte]
	metrics metrics
}

// Wrap adds a layer of in-memory caching to storage.Reader Get and Has operations.
// It returns an error if the capacity is less than or equal to zero or if the
// given store implements storage.Tx
func Wrap(store storage.IndexStore, capacity int) (*Cache, error) {
	lru, err := lru.New[string, []byte](capacity)
	if err != nil {
		return nil, err
	}

	return &Cache{store, lru, newMetrics()}, nil
}

// add caches given item.
func (c *Cache) add(i storage.Item) {
	b, err := i.Marshal()
	if err != nil {
		return
	}
	c.lru.Add(key(i), b)
}

// Get implements storage.Store interface.
// On a call it tries to first retrieve the item from cache.
// If the item does not exist in cache, it tries to retrieve
// it from the underlying store.
func (c *Cache) Get(i storage.Item) error {
	if val, ok := c.lru.Get(key(i)); ok {
		c.metrics.CacheHit.Inc()
		return i.Unmarshal(val)
	}

	if err := c.IndexStore.Get(i); err != nil {
		return err
	}

	c.metrics.CacheMiss.Inc()
	c.add(i)

	return nil
}

// Has implements storage.Store interface.
// On a call it tries to first retrieve the item from cache.
// If the item does not exist in cache, it tries to retrieve
// it from the underlying store.
func (c *Cache) Has(k storage.Key) (bool, error) {
	if _, ok := c.lru.Get(key(k)); ok {
		c.metrics.CacheHit.Inc()
		return true, nil
	}

	c.metrics.CacheMiss.Inc()
	return c.IndexStore.Has(k)
}

// Put implements storage.Store interface.
// On a call it also inserts the item into the cache so that the next
// call to Put and Has will be able to retrieve the item from cache.
func (c *Cache) Put(i storage.Item) error {
	c.add(i)
	return c.IndexStore.Put(i)
}

// Delete implements storage.Store interface.
// On a call it also removes the item from the cache.
func (c *Cache) Delete(i storage.Item) error {
	_ = c.lru.Remove(key(i))
	return c.IndexStore.Delete(i)
}

func (c *Cache) Close() error {
	c.lru.Purge()
	return nil
}
