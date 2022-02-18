// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemstore

import (
	"context"
	"sync"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// Store implements a simple Putter and Getter which can be used to temporarily cache
// chunks. Currently this is used in the bootstrapping process of new nodes where
// we sync the postage events from the swarm network.
type Store struct {
	mtx   sync.Mutex
	store map[string]swarm.Chunk
}

func New() *Store {
	return &Store{
		store: make(map[string]swarm.Chunk),
	}
}

func (s *Store) Get(_ context.Context, _ storage.ModeGet, addr swarm.Address) (ch swarm.Chunk, err error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if ch, ok := s.store[addr.ByteString()]; ok {
		return ch, nil
	}

	return nil, storage.ErrNotFound
}

func (s *Store) Put(_ context.Context, _ storage.ModePut, chs ...swarm.Chunk) (exist []bool, err error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	for _, ch := range chs {
		s.store[ch.Address().ByteString()] = ch
	}

	exist = make([]bool, len(chs))

	return exist, err
}

func (s *Store) GetMulti(_ context.Context, _ storage.ModeGet, _ ...swarm.Address) (ch []swarm.Chunk, err error) {
	panic("not implemented")
}

func (s *Store) Has(_ context.Context, _ swarm.Address) (yes bool, err error) {
	panic("not implemented")
}

func (s *Store) HasMulti(_ context.Context, _ ...swarm.Address) (yes []bool, err error) {
	panic("not implemented")
}

func (s *Store) Set(_ context.Context, _ storage.ModeSet, _ ...swarm.Address) (err error) {
	panic("not implemented")
}

func (s *Store) LastPullSubscriptionBinID(_ uint8) (id uint64, err error) {
	panic("not implemented")
}

func (s *Store) SubscribePull(_ context.Context, _ uint8, _ uint64, _ uint64) (c <-chan storage.Descriptor, closed <-chan struct{}, stop func()) {
	panic("not implemented")
}

func (s *Store) SubscribePush(_ context.Context, _ func([]byte) bool) (c <-chan swarm.Chunk, repeat func(), stop func()) {
	panic("not implemented")
}

func (s *Store) Close() error {
	return nil
}
