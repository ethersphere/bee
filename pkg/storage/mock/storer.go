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
)

var _ storage.Storer = (*MockStorer)(nil)

type MockStorer struct {
	store           map[string][]byte
	modeSet         map[string]storage.ModeSet
	modeSetMu       sync.Mutex
	pinnedAddress   []swarm.Address // Stores the pinned address
	pinnedCounter   []uint64        // and its respective counter. These are stored as slices to preserve the order.
	pinSetMu        sync.Mutex
	subpull         []storage.Descriptor
	partialInterval bool
	validator       swarm.ChunkValidator
	morePull        chan struct{}
	mtx             sync.Mutex
	quit            chan struct{}
}

func WithSubscribePullChunks(chs ...storage.Descriptor) Option {
	return optionFunc(func(m *MockStorer) {
		m.subpull = make([]storage.Descriptor, len(chs))
		for i, v := range chs {
			m.subpull[i] = v
		}
	})
}

func WithPartialInterval(v bool) Option {
	return optionFunc(func(m *MockStorer) {
		m.partialInterval = v
	})
}

func NewStorer(opts ...Option) *MockStorer {
	s := &MockStorer{
		store:     make(map[string][]byte),
		modeSet:   make(map[string]storage.ModeSet),
		modeSetMu: sync.Mutex{},
		morePull:  make(chan struct{}),
		quit:      make(chan struct{}),
	}

	for _, v := range opts {
		v.apply(s)
	}

	return s
}

func NewValidatingStorer(v swarm.ChunkValidator) *MockStorer {
	return &MockStorer{
		store:     make(map[string][]byte),
		modeSet:   make(map[string]storage.ModeSet),
		modeSetMu: sync.Mutex{},
		pinSetMu:  sync.Mutex{},
		validator: v,
	}
}

func (m *MockStorer) Get(ctx context.Context, mode storage.ModeGet, addr swarm.Address) (ch swarm.Chunk, err error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	v, has := m.store[addr.String()]
	if !has {
		return nil, storage.ErrNotFound
	}
	return swarm.NewChunk(addr, v), nil
}

func (m *MockStorer) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exist []bool, err error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

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

func (m *MockStorer) GetMulti(ctx context.Context, mode storage.ModeGet, addrs ...swarm.Address) (ch []swarm.Chunk, err error) {
	panic("not implemented") // TODO: Implement
}

func (m *MockStorer) Has(ctx context.Context, addr swarm.Address) (yes bool, err error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

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

func (m *MockStorer) SubscribePull(ctx context.Context, bin uint8, since, until uint64) (<-chan storage.Descriptor, <-chan struct{}, func()) {
	c := make(chan storage.Descriptor)
	done := make(chan struct{})
	stop := func() {
		close(done)
	}
	go func() {
		defer close(c)
		m.mtx.Lock()
		for _, ch := range m.subpull {
			select {
			case c <- ch:
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-m.quit:
				return
			}
		}
		m.mtx.Unlock()

		if m.partialInterval {
			// block since we're at the top of the bin and waiting for new chunks
			select {
			case <-done:
				return
			case <-m.quit:
				return
			case <-ctx.Done():
				return
			case <-m.morePull:

			}
		}

		m.mtx.Lock()
		defer m.mtx.Unlock()

		// iterate on what we have in the iterator
		for _, ch := range m.subpull {
			select {
			case c <- ch:
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-m.quit:
				return
			}
		}

	}()
	return c, m.quit, stop
}

func (m *MockStorer) MorePull(d ...storage.Descriptor) {
	// clear out what we already have in subpull
	m.mtx.Lock()
	defer m.mtx.Unlock()

	m.subpull = make([]storage.Descriptor, len(d))
	for i, v := range d {
		m.subpull[i] = v
	}
	close(m.morePull)
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
	close(m.quit)
	return nil
}

type Option interface {
	apply(*MockStorer)
}
type optionFunc func(*MockStorer)

func (f optionFunc) apply(r *MockStorer) { f(r) }
