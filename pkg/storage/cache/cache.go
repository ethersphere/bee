// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	"errors"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/hashicorp/golang-lru/v2"
)

var _ storage.Store = (*Cache)(nil)

// Cache is a wrapper around a storage.Store that adds a layer
// of in-memory caching for the Get and Has operations.
type Cache struct {
	storage.Store

	lru     *lru.Cache[string, []byte]
	metrics metrics
}

// Wrap adds a layer of in-memory caching to basic Store operations.
// This call will panic if the capacity is less than or equal to zero.
// It will also panic if the given store implements storage.Tx.
func Wrap(store storage.Store, capacity int) *Cache {
	if _, ok := store.(storage.Tx); ok {
		panic(errors.New("cache should not be used with transactions"))
	}

	lru, err := lru.New[string, []byte](capacity)
	if err != nil {
		panic(err)
	}

	return &Cache{store, lru, newMetrics()}
}

// add caches given item.
func (c *Cache) add(i storage.Item) {
	b, err := i.Marshal()
	if err != nil {
		return
	}
	c.lru.Add(i.ID(), b)
}

// Get implements storage.Store interface.
// On a call it tries to first retrieve the item from cache.
// If the item does not exist in cache, it tries to retrieve
// it from the underlying store.
func (c *Cache) Get(i storage.Item) error {
	if val, ok := c.lru.Get(i.ID()); ok {
		c.metrics.CacheHit.Inc()
		return i.Unmarshal(val)
	}

	if err := c.Store.Get(i); err != nil {
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
	if _, ok := c.lru.Get(k.ID()); ok {
		c.metrics.CacheHit.Inc()
		return true, nil
	}

	c.metrics.CacheMiss.Inc()
	return c.Store.Has(k)
}

// Put implements storage.Store interface.
// On a call it also inserts the item into the cache so that the next
// call to Put and Has will be able to retrieve the item from cache.
func (c *Cache) Put(i storage.Item) error {
	c.add(i)
	return c.Store.Put(i)
}

// Delete implements storage.Store interface.
// On a call it also removes the item from the cache.
func (c *Cache) Delete(i storage.Item) error {
	_ = c.lru.Remove(i.ID())
	return c.Store.Delete(i)
}
