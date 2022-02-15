package inmemstore

import (
	"context"
	"sync"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type Store struct {
	mtx   sync.Mutex
	store map[string]swarm.Chunk
}

func New() *Store {
	return &Store{
		store: make(map[string]swarm.Chunk),
	}
}

func (s *Store) Get(ctx context.Context, mode storage.ModeGet, addr swarm.Address) (ch swarm.Chunk, err error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if ch, ok := s.store[addr.ByteString()]; ok {
		return ch, nil
	}

	return nil, storage.ErrNotFound
}

func (s *Store) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) (exist []bool, err error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	for _, ch := range chs {
		s.store[ch.Address().ByteString()] = ch
	}

	exist = make([]bool, len(chs))

	return exist, err
}

func (s *Store) GetMulti(ctx context.Context, mode storage.ModeGet, addrs ...swarm.Address) (ch []swarm.Chunk, err error) {
	panic("not implemented")
}

func (s *Store) Has(ctx context.Context, addr swarm.Address) (yes bool, err error) {
	panic("not implemented")
}

func (s *Store) HasMulti(ctx context.Context, addrs ...swarm.Address) (yes []bool, err error) {
	panic("not implemented")
}

func (s *Store) Set(ctx context.Context, mode storage.ModeSet, addrs ...swarm.Address) (err error) {
	panic("not implemented")
}

func (s *Store) LastPullSubscriptionBinID(bin uint8) (id uint64, err error) {
	panic("not implemented")
}

func (s *Store) SubscribePull(ctx context.Context, bin uint8, since uint64, until uint64) (c <-chan storage.Descriptor, closed <-chan struct{}, stop func()) {
	panic("not implemented")
}

func (s *Store) SubscribePush(ctx context.Context, skipf func([]byte) bool) (c <-chan swarm.Chunk, repeat func(), stop func()) {
	panic("not implemented")
}

func (s *Store) Close() error {
	return nil
}
