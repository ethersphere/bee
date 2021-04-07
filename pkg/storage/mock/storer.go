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
	store           map[string]swarm.Chunk
	modePut         map[string]storage.ModePut
	modeSet         map[string]storage.ModeSet
	pinnedAddress   []swarm.Address // Stores the pinned address
	pinnedCounter   []uint64        // and its respective counter. These are stored as slices to preserve the order.
	subpull         []storage.Descriptor
	partialInterval bool
	morePull        chan struct{}
	mtx             sync.Mutex
	quit            chan struct{}
	baseAddress     []byte
	bins            []uint64
}

func WithSubscribePullChunks(chs ...storage.Descriptor) Option {
	return optionFunc(func(m *MockStorer) {
		m.subpull = make([]storage.Descriptor, len(chs))
		for i, v := range chs {
			m.subpull[i] = v
		}
	})
}

func WithBaseAddress(a swarm.Address) Option {
	return optionFunc(func(m *MockStorer) {
		m.baseAddress = a.Bytes()
	})
}

func WithPartialInterval(v bool) Option {
	return optionFunc(func(m *MockStorer) {
		m.partialInterval = v
	})
}

func NewStorer(opts ...Option) *MockStorer {
	s := &MockStorer{
		store:    make(map[string]swarm.Chunk),
		modePut:  make(map[string]storage.ModePut),
		modeSet:  make(map[string]storage.ModeSet),
		morePull: make(chan struct{}),
		quit:     make(chan struct{}),
		bins:     make([]uint64, swarm.MaxBins),
	}

	for _, v := range opts {
		v.apply(s)
	}

	return s
}

func (m *MockStorer) Get(_ context.Context, _ storage.ModeGet, addr swarm.Address) (ch swarm.Chunk, err error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	v, has := m.store[addr.String()]
	if !has {
		return nil, storage.ErrNotFound
	}
	return v, nil
}

func (m *MockStorer) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exist []bool, err error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	exist = make([]bool, len(chs))
	for i, ch := range chs {
		exist[i], err = m.has(ctx, ch.Address())
		if err != nil {
			return exist, err
		}
		if !exist[i] {
			po := swarm.Proximity(ch.Address().Bytes(), m.baseAddress)
			m.bins[po]++
		}
		m.store[ch.Address().String()] = ch
		m.modePut[ch.Address().String()] = mode

		// pin chunks if needed
		switch mode {
		case storage.ModePutUploadPin:
			// if mode is set pin, increment the pin counter
			var found bool
			addr := ch.Address()
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
		default:
		}
	}
	return exist, nil
}

func (m *MockStorer) GetMulti(ctx context.Context, mode storage.ModeGet, addrs ...swarm.Address) (ch []swarm.Chunk, err error) {
	panic("not implemented") // TODO: Implement
}

func (m *MockStorer) has(ctx context.Context, addr swarm.Address) (yes bool, err error) {
	_, has := m.store[addr.String()]
	return has, nil
}

func (m *MockStorer) Has(ctx context.Context, addr swarm.Address) (yes bool, err error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	return m.has(ctx, addr)
}

func (m *MockStorer) HasMulti(ctx context.Context, addrs ...swarm.Address) (yes []bool, err error) {
	panic("not implemented") // TODO: Implement
}

func (m *MockStorer) Set(ctx context.Context, mode storage.ModeSet, addrs ...swarm.Address) (err error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	for _, addr := range addrs {
		m.modeSet[addr.String()] = mode
		switch mode {
		case storage.ModeSetPin:
			// check if chunk exists
			has, err := m.has(ctx, addr)
			if err != nil {
				return err
			}

			if !has {
				return storage.ErrNotFound
			}

			// if mode is set pin, increment the pin counter
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
		case storage.ModeSetUnpin:
			// if mode is set unpin, decrement the pin counter and remove the address
			// once it reaches zero
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
		case storage.ModeSetRemove:
			delete(m.store, addr.String())
		default:
		}
	}
	return nil
}
func (m *MockStorer) GetModePut(addr swarm.Address) (mode storage.ModePut) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if mode, ok := m.modePut[addr.String()]; ok {
		return mode
	}
	return mode
}

func (m *MockStorer) GetModeSet(addr swarm.Address) (mode storage.ModeSet) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if mode, ok := m.modeSet[addr.String()]; ok {
		return mode
	}
	return mode
}

func (m *MockStorer) LastPullSubscriptionBinID(bin uint8) (id uint64, err error) {
	return m.bins[bin], nil
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

func (m *MockStorer) PinnedChunks(ctx context.Context, offset, cursor int) (pinnedChunks []*storage.Pinner, err error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if len(m.pinnedAddress) == 0 {
		return pinnedChunks, nil
	}
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

func (m *MockStorer) PinCounter(address swarm.Address) (uint64, error) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
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
