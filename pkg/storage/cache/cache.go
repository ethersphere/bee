// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	"github.com/ethersphere/bee/pkg/storage"
	lru "github.com/hashicorp/golang-lru/v2"
)

var _ storage.Store = (*Cache)(nil)

type Cache struct {
	s       storage.Store
	c       *lru.Cache[string, []byte]
	metrics metrics
}

// MemCaching adds a layer of in-memory caching to basic Store operations.
// Should NOT be used in cases where transactions are involved.
func MemCaching(store storage.Store, capacity int) *Cache {
	c, _ := lru.New[string, []byte](capacity)
	return &Cache{store, c, newMetrics()}
}

func (c *Cache) Get(i storage.Item) error {
	if val, ok := c.c.Get(i.ID()); ok {
		c.metrics.CacheHit.Inc()
		return i.Unmarshal(val)
	}

	err := c.s.Get(i)
	if err != nil {
		return err
	}

	c.metrics.CacheMiss.Inc()
	c.addCache(i)

	return nil
}

func (c *Cache) Has(k storage.Key) (bool, error) {
	if _, ok := c.c.Get(k.ID()); ok {
		c.metrics.CacheHit.Inc()
		return true, nil
	}
	c.metrics.CacheMiss.Inc()
	return c.s.Has(k)
}

func (c *Cache) GetSize(k storage.Key) (int, error) {
	return c.s.GetSize(k)
}

func (c *Cache) Iterate(q storage.Query, f storage.IterateFn) error {
	return c.s.Iterate(q, f)
}

func (c *Cache) Count(k storage.Key) (int, error) {
	return c.s.Count(k)
}

func (c *Cache) Put(i storage.Item) error {
	c.addCache(i)
	return c.s.Put(i)
}

func (c *Cache) Delete(i storage.Item) error {
	_ = c.c.Remove(i.ID())
	return c.s.Delete(i)
}

func (c *Cache) Close() error {
	return c.s.Close()
}

func (c *Cache) addCache(i storage.Item) {
	b, err := i.Marshal()
	if err != nil {
		return
	}
	c.c.Add(i.ID(), b)
}
