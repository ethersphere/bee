// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handshake

import (
	"sync"

	"github.com/ethersphere/bee/v2/pkg/bzz"
)

// addressCache stores the most recently minted session-stable signed
// BzzAddress and the key (chequebook + underlay set) it was minted for. A key
// change makes the previous record obsolete, so only the latest one is kept.
//
// Minting runs inside the mutex, which guarantees:
//
//   - single flight: concurrent lookups for the same key mint exactly once;
//   - monotonic timestamps: each mint is strictly newer than the last, even
//     when the wall clock repeats a second or steps back. Peers reject
//     records older than the one they hold (see bzz.CheckTimestamp).
type addressCache struct {
	mu     sync.Mutex
	key    string
	addr   *bzz.Address
	lastTS int64 // timestamp of the most recently minted address
}

// getOrMint returns the cached address on a key match, otherwise mints with
// the next monotonic timestamp, max(now, last+1), and caches the result. A
// failed mint does not consume the timestamp.
func (c *addressCache) getOrMint(key string, now int64, mint func(timestamp int64) (*bzz.Address, error)) (*bzz.Address, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.addr != nil && c.key == key {
		return c.addr, nil
	}

	timestamp := max(now, c.lastTS+1)

	addr, err := mint(timestamp)
	if err != nil {
		return nil, err
	}
	c.lastTS = timestamp
	c.key, c.addr = key, addr

	return addr, nil
}
