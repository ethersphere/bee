// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage

import (
	lru "github.com/hashicorp/golang-lru/v2"
)

var _ Store = (*Cache)(nil)

type Cache struct {
	s Store
	c *lru.Cache[string, []byte]
}

// MemCaching adds a layer of in-memory caching to basic Store operations.
// Should NOT be used in cases where transactions are involved.
func MemCaching(store Store, capacity int) *Cache {
	c, _ := lru.New[string, []byte](capacity)
	return &Cache{store, c}
}

func (c *Cache) Get(i Item) error {
	if val, ok := c.c.Get(i.ID()); ok {
		return i.Unmarshal(val)
	}

	err := c.s.Get(i)
	if err != nil {
		return err
	}

	c.addCache(i)

	return nil
}

func (c *Cache) Has(k Key) (bool, error) {
	if _, ok := c.c.Get(k.ID()); ok {
		return true, nil
	}
	return c.s.Has(k)
}

func (c *Cache) GetSize(k Key) (int, error) {
	return c.s.GetSize(k)
}

func (c *Cache) Iterate(q Query, f IterateFn) error {
	return c.s.Iterate(q, f)
}

func (c *Cache) Count(k Key) (int, error) {
	return c.s.Count(k)
}

func (c *Cache) Put(i Item) error {
	c.addCache(i)
	return c.s.Put(i)
}

func (c *Cache) Delete(i Item) error {
	_ = c.c.Remove(i.ID())
	return c.s.Delete(i)
}

func (c *Cache) Close() error {
	return c.s.Close()
}

func (c *Cache) addCache(i Item) {
	b, err := i.Marshal()
	if err != nil {
		return
	}
	c.c.Add(i.ID(), b)
}
