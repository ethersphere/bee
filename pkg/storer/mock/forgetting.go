// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mockstorer

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type DelayedStore struct {
	storage.ChunkStore
	cache map[string]time.Duration
	mu    sync.Mutex
}

func NewDelayedStore(s storage.ChunkStore) *DelayedStore {
	return &DelayedStore{
		ChunkStore: s,
		cache:      make(map[string]time.Duration),
	}
}

func (d *DelayedStore) Delay(addr swarm.Address, delay time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cache[addr.String()] = delay
}

func (d *DelayedStore) Get(ctx context.Context, addr swarm.Address) (ch swarm.Chunk, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if delay, ok := d.cache[addr.String()]; ok && delay > 0 {
		select {
		case <-time.After(delay):
			delete(d.cache, addr.String())
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return d.ChunkStore.Get(ctx, addr)
}

type ForgettingStore struct {
	*DelayedStore
	oneIn int
	delay time.Duration
	n     atomic.Uint32
}

func NewForgettingStore(s storage.ChunkStore, oneIn int, delay time.Duration) *ForgettingStore {
	return &ForgettingStore{
		DelayedStore: NewDelayedStore(s),
		oneIn:        oneIn,
		delay:        delay,
	}
}

func (f *ForgettingStore) Put(ctx context.Context, ch swarm.Chunk) (err error) {
	if f.oneIn > 0 && int(f.n.Add(1))%f.oneIn == 0 {
		f.DelayedStore.Delay(ch.Address(), f.delay)
	}
	return f.DelayedStore.Put(ctx, ch)
}
