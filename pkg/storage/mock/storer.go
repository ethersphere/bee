// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"sync"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type MockStorer interface {
	storage.Storer
	GetModeSet(addr swarm.Address) (mode storage.ModeSet)
}

type mockStorer struct {
	store     map[string][]byte
	modeSet   map[string]storage.ModeSet
	modeSetMu *sync.Mutex
	validator swarm.ChunkValidator
}

func NewStorer() storage.Storer {
	return &mockStorer{
		store:     make(map[string][]byte),
		modeSet:   make(map[string]storage.ModeSet),
		modeSetMu: &sync.Mutex{},
	}
}

func NewValidatingStorer(v swarm.ChunkValidator) MockStorer {
	return &mockStorer{
		store:     make(map[string][]byte),
		modeSet:   make(map[string]storage.ModeSet),
		modeSetMu: &sync.Mutex{},
		validator: v,
	}
}

func (m *mockStorer) Get(ctx context.Context, mode storage.ModeGet, addr swarm.Address) (ch swarm.Chunk, err error) {
	v, has := m.store[addr.String()]
	if !has {
		return nil, storage.ErrNotFound
	}
	return swarm.NewChunk(addr, v), nil
}

func (m *mockStorer) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exist []bool, err error) {
	for _, ch := range chs {
		if m.validator != nil {
			if !m.validator.Validate(ch) {
				return nil, storage.ErrInvalidChunk
			}
		}
		m.store[ch.Address().String()] = ch.Data()
	}
	return nil, nil
}

func (m *mockStorer) GetMulti(ctx context.Context, mode storage.ModeGet, addrs ...swarm.Address) (ch []swarm.Chunk, err error) {
	panic("not implemented") // TODO: Implement
}

func (m *mockStorer) Has(ctx context.Context, addr swarm.Address) (yes bool, err error) {
	_, has := m.store[addr.String()]
	return has, nil
}

func (m *mockStorer) HasMulti(ctx context.Context, addrs ...swarm.Address) (yes []bool, err error) {
	panic("not implemented") // TODO: Implement
}

func (m *mockStorer) Set(ctx context.Context, mode storage.ModeSet, addrs ...swarm.Address) (err error) {
	m.modeSetMu.Lock()
	defer m.modeSetMu.Unlock()
	for _, addr := range addrs {
		m.modeSet[addr.String()] = mode
	}
	return nil
}

func (m *mockStorer) GetModeSet(addr swarm.Address) (mode storage.ModeSet) {
	m.modeSetMu.Lock()
	defer m.modeSetMu.Unlock()
	if mode, ok := m.modeSet[addr.String()]; ok {
		return mode
	}
	return mode
}

func (m *mockStorer) LastPullSubscriptionBinID(bin uint8) (id uint64, err error) {
	panic("not implemented") // TODO: Implement
}

func (m *mockStorer) SubscribePull(ctx context.Context, bin uint8, since uint64, until uint64) (c <-chan storage.Descriptor, stop func()) {
	panic("not implemented") // TODO: Implement
}

func (m *mockStorer) SubscribePush(ctx context.Context) (c <-chan swarm.Chunk, stop func()) {
	panic("not implemented") // TODO: Implement
}

func (m *mockStorer) GetPinnedChunks(ctx context.Context, cursor swarm.Address) (pinnedChunks []*storage.PinInfo, err error) {
	panic("not implemented") // TODO: Implement
}

func (m *mockStorer) GetPinInfo(address swarm.Address) (uint64, error) {
	panic("not implemented") // TODO: Implement
}

func (m *mockStorer) Close() error {
	panic("not implemented") // TODO: Implement
}
