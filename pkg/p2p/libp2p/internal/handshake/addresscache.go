// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
	"container/list"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/bzz"
)

// addressCache stores the session-stable signed BzzAddress per cache key,
// evicting the least recently used entry when capacity is exceeded.
//
// The cache owns the mutex and runs mint inside it, which guarantees two
// invariants no per-method locking could:
//
//   - single flight: concurrent lookups for the same key produce exactly
//     one signed record;
//   - monotonic timestamps: every minted record carries a strictly greater
//     timestamp than the previous one, across all keys, even when the wall
//     clock repeats a second or steps backwards.
type addressCache struct {
	mu      sync.Mutex
	cap     int
	entries map[string]*list.Element
	lru     *list.List // *cacheEntry elements, most recently used in front
	lastTS  int64      // timestamp of the most recently minted address
}

// cacheEntry pairs the cache key with the signed address minted for it.
type cacheEntry struct {
	key  string
	addr *bzz.Address
}

func newAddressCache(capacity int) *addressCache {
	return &addressCache{
		cap:     capacity,
		entries: make(map[string]*list.Element),
		lru:     list.New(),
	}
}

// getOrMint returns the cached address for key, or calls mint exactly once
// with the next monotonic timestamp, max(now, last+1), and caches the result.
// A failed mint does not consume the timestamp.
func (c *addressCache) getOrMint(key string, now int64, mint func(timestamp int64) (*bzz.Address, error)) (*bzz.Address, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.entries[key]; ok {
		c.lru.MoveToFront(el)
		return el.Value.(*cacheEntry).addr, nil
	}

	timestamp := max(now, c.lastTS+1)

	addr, err := mint(timestamp)
	if err != nil {
		return nil, err
	}
	c.lastTS = timestamp

	c.entries[key] = c.lru.PushFront(&cacheEntry{key: key, addr: addr})
	if c.lru.Len() > c.cap {
		oldest := c.lru.Back()
		c.lru.Remove(oldest)
		delete(c.entries, oldest.Value.(*cacheEntry).key)
	}

	return addr, nil
}

// purge drops all cached addresses; the next getOrMint per key re-signs.
func (c *addressCache) purge() {
	c.mu.Lock()
	defer c.mu.Unlock()

	clear(c.entries)
	c.lru.Init()
}

// size returns the number of cached addresses.
func (c *addressCache) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.lru.Len()
}
