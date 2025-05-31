// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/storageutil"
)

// key returns a string representation of the given key.
func key(key storage.Key) string {
	return storageutil.JoinFields(key.Namespace(), key.ID())
}

var _ storage.IndexStore = (*Cache)(nil)

// add caches given item.
func (c *Cache) add(i storage.Item) {
	b, err := i.Marshal()
	if err != nil {
		return
	}
	c.lru.Add(key(i), b)
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
