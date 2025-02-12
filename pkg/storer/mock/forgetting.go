// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mockstorer

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
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
	delay, ok := d.cache[addr.String()]
	if ok && delay > 0 {
		delete(d.cache, addr.String())
		d.mu.Unlock()
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	} else {
		d.mu.Unlock()
	}
	return d.ChunkStore.Get(ctx, addr)
}

type ForgettingStore struct {
	storage.ChunkStore
	record atomic.Bool
	mu     sync.Mutex
	n      atomic.Int64
	missed map[string]struct{}
}

func NewForgettingStore(s storage.ChunkStore) *ForgettingStore {
	return &ForgettingStore{ChunkStore: s, missed: make(map[string]struct{})}
}

func (f *ForgettingStore) Stored() int64 {
	return f.n.Load()
}

func (f *ForgettingStore) Record() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record.Store(true)
}

func (f *ForgettingStore) Unrecord() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record.Store(false)
}

func (f *ForgettingStore) Miss(addr swarm.Address) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.miss(addr)
}

func (f *ForgettingStore) Unmiss(addr swarm.Address) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unmiss(addr)
}

func (f *ForgettingStore) miss(addr swarm.Address) {
	f.missed[addr.String()] = struct{}{}
}

func (f *ForgettingStore) unmiss(addr swarm.Address) {
	delete(f.missed, addr.String())
}

func (f *ForgettingStore) isMiss(addr swarm.Address) bool {
	_, ok := f.missed[addr.String()]
	return ok
}

func (f *ForgettingStore) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.missed = make(map[string]struct{})
}

func (f *ForgettingStore) Missed() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.missed)
}

// Get implements the ChunkStore interface.
// if in recording phase, record the chunk address as miss and returns Get on the embedded store
// if in forgetting phase, returns ErrNotFound if the chunk address is recorded as miss
func (f *ForgettingStore) Get(ctx context.Context, addr swarm.Address) (ch swarm.Chunk, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.record.Load() {
		f.miss(addr)
	} else if f.isMiss(addr) {
		return nil, storage.ErrNotFound
	}
	return f.ChunkStore.Get(ctx, addr)
}

// Put implements the ChunkStore interface.
func (f *ForgettingStore) Put(ctx context.Context, ch swarm.Chunk) (err error) {
	f.n.Add(1)
	if !f.record.Load() {
		f.Unmiss(ch.Address())
	}
	return f.ChunkStore.Put(ctx, ch)
}
