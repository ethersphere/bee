// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"errors"
	"sync"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
)

var _ storage.Storer = (*MockStorer)(nil)

type MockStorer struct {
	store         map[string][]byte
	modeSet       map[string]storage.ModeSet
	modeSetMu     sync.Mutex
	pinnedAddress []swarm.Address // Stores the pinned address
	pinnedCounter []uint64        // and its respective counter. These are stored as slices to preserve the order.
	pinSetMu      sync.Mutex
	validator     swarm.ChunkValidator
	tags          *tags.Tags
}

func NewStorer() storage.Storer {
	return &MockStorer{
		store:     make(map[string][]byte),
		modeSet:   make(map[string]storage.ModeSet),
		modeSetMu: sync.Mutex{},
	}
}

func NewValidatingStorer(v swarm.ChunkValidator, tags *tags.Tags) *MockStorer {
	return &MockStorer{
		store:     make(map[string][]byte),
		modeSet:   make(map[string]storage.ModeSet),
		modeSetMu: sync.Mutex{},
		pinSetMu:  sync.Mutex{},
		validator: v,
		tags:      tags,
	}
}

func (m *MockStorer) Get(ctx context.Context, mode storage.ModeGet, addr swarm.Address) (ch swarm.Chunk, err error) {
	v, has := m.store[addr.String()]
	if !has {
		return nil, storage.ErrNotFound
	}
	return swarm.NewChunk(addr, v), nil
}

func (m *MockStorer) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exist []bool, err error) {
	for _, ch := range chs {
		if m.validator != nil {
			if !m.validator.Validate(ch) {
				return nil, storage.ErrInvalidChunk
			}
		}
		m.store[ch.Address().String()] = ch.Data()
		yes, err := m.Has(ctx, ch.Address())
		if err != nil {
			exist = append(exist, false)
			continue
		}
		if yes {
			exist = append(exist, true)
		} else {
			exist = append(exist, false)
		}

	}
	return exist, nil
}

func (m *MockStorer) GetMulti(ctx context.Context, mode storage.ModeGet, addrs ...swarm.Address) (ch []swarm.Chunk, err error) {
	panic("not implemented") // TODO: Implement
}

func (m *MockStorer) Has(ctx context.Context, addr swarm.Address) (yes bool, err error) {
	_, has := m.store[addr.String()]
	return has, nil
}

func (m *MockStorer) HasMulti(ctx context.Context, addrs ...swarm.Address) (yes []bool, err error) {
	panic("not implemented") // TODO: Implement
}

func (m *MockStorer) Set(ctx context.Context, mode storage.ModeSet, addrs ...swarm.Address) (err error) {
	m.modeSetMu.Lock()
	m.pinSetMu.Lock()
	defer m.modeSetMu.Unlock()
	defer m.pinSetMu.Unlock()
	for _, addr := range addrs {
		m.modeSet[addr.String()] = mode

		// if mode is set pin, increment the pin counter
		if mode == storage.ModeSetPin {
			var found bool
			for i, ad := range m.pinnedAddress {
				if addr.String() == ad.String() {
					m.pinnedCounter[i] = m.pinnedCounter[i] + 1
					found = true
				}
			}
			if !found {
				m.pinnedAddress = append(m.pinnedAddress, addr)
				m.pinnedCounter = append(m.pinnedCounter, uint64(1))
			}
		}

		// if mode is set unpin, decrement the pin counter and remove the address
		// once it reaches zero
		if mode == storage.ModeSetUnpin {
			for i, ad := range m.pinnedAddress {
				if addr.String() == ad.String() {
					m.pinnedCounter[i] = m.pinnedCounter[i] - 1
					if m.pinnedCounter[i] == 0 {
						copy(m.pinnedAddress[i:], m.pinnedAddress[i+1:])
						m.pinnedAddress[len(m.pinnedAddress)-1] = swarm.NewAddress([]byte{0})
						m.pinnedAddress = m.pinnedAddress[:len(m.pinnedAddress)-1]

						copy(m.pinnedCounter[i:], m.pinnedCounter[i+1:])
						m.pinnedCounter[len(m.pinnedCounter)-1] = uint64(0)
						m.pinnedCounter = m.pinnedCounter[:len(m.pinnedCounter)-1]
					}
				}
			}
		}
	}
	return nil
}

func (m *MockStorer) GetModeSet(addr swarm.Address) (mode storage.ModeSet) {
	m.modeSetMu.Lock()
	defer m.modeSetMu.Unlock()
	if mode, ok := m.modeSet[addr.String()]; ok {
		return mode
	}
	return mode
}

func (m *MockStorer) LastPullSubscriptionBinID(bin uint8) (id uint64, err error) {
	panic("not implemented") // TODO: Implement
}

func (m *MockStorer) SubscribePull(ctx context.Context, bin uint8, since uint64, until uint64) (c <-chan storage.Descriptor, stop func()) {
	panic("not implemented") // TODO: Implement
}

func (m *MockStorer) SubscribePush(ctx context.Context) (c <-chan swarm.Chunk, stop func()) {
	panic("not implemented") // TODO: Implement
}

func (m *MockStorer) PinnedChunks(ctx context.Context, cursor swarm.Address) (pinnedChunks []*storage.Pinner, err error) {
	m.pinSetMu.Lock()
	defer m.pinSetMu.Unlock()
	for i, addr := range m.pinnedAddress {
		pi := &storage.Pinner{
			Address:    swarm.NewAddress(addr.Bytes()),
			PinCounter: m.pinnedCounter[i],
		}
		pinnedChunks = append(pinnedChunks, pi)
	}
	if pinnedChunks == nil {
		return pinnedChunks, errors.New("pin chunks: leveldb: not found")
	}
	return pinnedChunks, nil
}

func (m *MockStorer) PinInfo(address swarm.Address) (uint64, error) {
	m.pinSetMu.Lock()
	defer m.pinSetMu.Unlock()
	for i, addr := range m.pinnedAddress {
		if addr.String() == address.String() {
			return m.pinnedCounter[i], nil
		}
	}
	return 0, storage.ErrNotFound
}

func (m *MockStorer) Close() error {
	panic("not implemented") // TODO: Implement
}
