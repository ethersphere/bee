// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	"context"
	"fmt"
	"time"

	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type (
	CacheEntry = cacheEntry
)

var (
	ErrMarshalCacheEntryInvalidAddress   = errMarshalCacheEntryInvalidAddress
	ErrMarshalCacheEntryInvalidTimestamp = errMarshalCacheEntryInvalidTimestamp
	ErrUnmarshalCacheEntryInvalidSize    = errUnmarshalCacheEntryInvalidSize
)

func ReplaceTimeNow(fn func() time.Time) func() {
	now = fn
	return func() {
		now = time.Now
	}
}

type CacheState struct {
	Head swarm.Address
	Tail swarm.Address
	Size uint64
}

func (c *Cache) RemoveOldestMaxBatch(ctx context.Context, st transaction.Storage, count uint64, batchCnt int) error {
	return c.RemoveOldest(ctx, st, count)
}

func (c *Cache) State(store storage.Reader) CacheState {
	state := CacheState{}
	state.Size = c.Size()
	runner := swarm.ZeroAddress

	err := store.Iterate(
		storage.Query{
			Factory:      func() storage.Item { return &cacheOrderIndex{} },
			ItemProperty: storage.QueryItemID,
		},
		func(res storage.Result) (bool, error) {
			_, addr, err := idFromKey(res.ID)
			if err != nil {
				return false, err
			}

			if state.Head.Equal(swarm.ZeroAddress) {
				state.Head = addr
			}
			runner = addr
			return false, nil
		},
	)
	if err != nil {
		panic(err)
	}
	state.Tail = runner
	return state
}

func (c *Cache) IterateOldToNew(
	st storage.Reader,
	start, end swarm.Address,
	iterateFn func(ch swarm.Address) (bool, error),
) error {
	runner := swarm.ZeroAddress
	err := st.Iterate(
		storage.Query{
			Factory:      func() storage.Item { return &cacheOrderIndex{} },
			ItemProperty: storage.QueryItemID,
		},
		func(res storage.Result) (bool, error) {
			_, addr, err := idFromKey(res.ID)
			if err != nil {
				return false, err
			}

			if runner.Equal(swarm.ZeroAddress) {
				if !addr.Equal(start) {
					return false, fmt.Errorf("invalid cache order index key %s", res.ID)
				}
			}
			runner = addr
			return iterateFn(runner)
		},
	)
	if err != nil {
		return err
	}
	if !runner.Equal(end) {
		return fmt.Errorf("invalid cache order index key %s", runner.String())
	}

	return nil
}
