// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type (
	CacheEntry = cacheEntry
	CacheState = cacheState
)

var (
	ErrUnmarshalCacheStateInvalidSize  = errUnmarshalCacheStateInvalidSize
	ErrMarshalCacheEntryInvalidAddress = errMarshalCacheEntryInvalidAddress
	ErrUnmarshalCacheEntryInvalidSize  = errUnmarshalCacheEntryInvalidSize
)

func (c *Cache) State(store storage.Store) CacheState {
	state := &CacheState{}
	err := store.Get(state)
	if err != nil {
		return CacheState{}
	}
	return *state
}

func (c *Cache) IterateOldToNew(
	st storage.Store,
	start, end swarm.Address,
	iterateFn func(ch swarm.Address) (bool, error),
) error {

	currentAddr := start
	for !currentAddr.Equal(end) {
		entry := &cacheEntry{Address: currentAddr}
		err := st.Get(entry)
		if err != nil {
			return err
		}
		stop, err := iterateFn(entry.Address)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
		currentAddr = entry.Next
	}

	return nil
}
