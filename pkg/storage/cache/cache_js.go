//go:build js
// +build js

package cache

import (
	"github.com/ethersphere/bee/v2/pkg/storage"
	lru "github.com/hashicorp/golang-lru/v2"
)

// Cache is a wrapper around a storage.Store that adds a layer
// of in-memory caching for the Get and Has operations.
type Cache struct {
	storage.IndexStore

	lru *lru.Cache[string, []byte]
}

// Wrap adds a layer of in-memory caching to storage.Reader Get and Has operations.
// It returns an error if the capacity is less than or equal to zero or if the
// given store implements storage.Tx
func Wrap(store storage.IndexStore, capacity int) (*Cache, error) {
	lru, err := lru.New[string, []byte](capacity)
	if err != nil {
		return nil, err
	}

	return &Cache{store, lru}, nil
}

// Get implements storage.Store interface.
// On a call it tries to first retrieve the item from cache.
// If the item does not exist in cache, it tries to retrieve
// it from the underlying store.
func (c *Cache) Get(i storage.Item) error {
	if val, ok := c.lru.Get(key(i)); ok {
		return i.Unmarshal(val)
	}

	if err := c.IndexStore.Get(i); err != nil {
		return err
	}

	c.add(i)

	return nil
}

// Has implements storage.Store interface.
// On a call it tries to first retrieve the item from cache.
// If the item does not exist in cache, it tries to retrieve
// it from the underlying store.
func (c *Cache) Has(k storage.Key) (bool, error) {
	if _, ok := c.lru.Get(key(k)); ok {

		return true, nil
	}

	return c.IndexStore.Has(k)
}
