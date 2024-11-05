// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mockstorer

import (
	"context"
	"math/big"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type chunksResponse struct {
	chunks []*storer.BinC
	err    error
}

// WithSubscribeResp mocks a desired response when calling IntervalChunks method.
// Different possible responses for subsequent responses in multi-call scenarios
// are possible (i.e. first call yields a,b,c, second call yields d,e,f).
// Mock maintains state of current call using chunksCalls counter.
func WithSubscribeResp(chunks []*storer.BinC, err error) Option {
	return optionFunc(func(p *ReserveStore) {
		p.subResponses = append(p.subResponses, chunksResponse{chunks: chunks, err: err})
	})
}

// WithChunks mocks the set of chunks that the store is aware of (used in Get and Has calls).
func WithChunks(chs ...swarm.Chunk) Option {
	return optionFunc(func(p *ReserveStore) {
		for _, c := range chs {
			if c.Stamp() != nil {
				stampHash, _ := c.Stamp().Hash()
				p.chunks[c.Address().String()+string(c.Stamp().BatchID())+string(stampHash)] = c
			} else {
				p.chunks[c.Address().String()] = c
			}
		}
	})
}

// WithEvilChunk allows to inject a malicious chunk (request a certain address
// of a chunk, but get another), in order to mock unsolicited chunk delivery.
func WithEvilChunk(addr swarm.Address, ch swarm.Chunk) Option {
	return optionFunc(func(p *ReserveStore) {
		p.evilAddr = addr
		p.evilChunk = ch
	})
}

func WithCursors(c []uint64, e uint64) Option {
	return optionFunc(func(p *ReserveStore) {
		p.cursors = c
		p.epoch = e
	})
}

func WithCursorsErr(e error) Option {
	return optionFunc(func(p *ReserveStore) {
		p.cursorsErr = e
	})
}

func WithRadius(r uint8) Option {
	return optionFunc(func(p *ReserveStore) {
		p.radius = r
	})
}

func WithReserveSize(s int) Option {
	return optionFunc(func(p *ReserveStore) {
		p.reservesize = s
	})
}

func WithCapacityDoubling(s int) Option {
	return optionFunc(func(p *ReserveStore) {
		p.capacityDoubling = s
	})
}

func WithPutHook(f func(swarm.Chunk) error) Option {
	return optionFunc(func(p *ReserveStore) {
		p.putHook = f
	})
}

func WithSample(s storer.Sample) Option {
	return optionFunc(func(p *ReserveStore) {
		p.sample = s
	})
}

var _ storer.ReserveStore = (*ReserveStore)(nil)

type ReserveStore struct {
	mtx         sync.Mutex
	chunksCalls int
	putCalls    int
	setCalls    int

	chunks    map[string]swarm.Chunk
	evilAddr  swarm.Address
	evilChunk swarm.Chunk

	cursors    []uint64
	cursorsErr error
	epoch      uint64

	radius           uint8
	reservesize      int
	capacityDoubling int

	subResponses []chunksResponse
	putHook      func(swarm.Chunk) error

	sample storer.Sample
}

// NewReserve returns a new Reserve mock.
func NewReserve(opts ...Option) *ReserveStore {
	s := &ReserveStore{
		chunks: make(map[string]swarm.Chunk),
	}
	for _, v := range opts {
		v.apply(s)
	}
	return s
}

func (s *ReserveStore) EvictBatch(ctx context.Context, batchID []byte) error { return nil }
func (s *ReserveStore) IsWithinStorageRadius(addr swarm.Address) bool        { return true }
func (s *ReserveStore) IsFullySynced() bool                                  { return true }
func (s *ReserveStore) StorageRadius() uint8 {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.radius
}

func (s *ReserveStore) SetStorageRadius(r uint8) {
	s.mtx.Lock()
	s.radius = r
	s.mtx.Unlock()
}
func (s *ReserveStore) CommittedDepth() uint8 {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.radius + uint8(s.capacityDoubling)
}

// IntervalChunks returns a set of chunk in a requested interval.
func (s *ReserveStore) SubscribeBin(ctx context.Context, bin uint8, start uint64) (<-chan *storer.BinC, func(), <-chan error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	r := s.subResponses[s.chunksCalls]
	s.chunksCalls++

	out := make(chan *storer.BinC)
	errC := make(chan error, 1)

	go func() {
		for _, c := range r.chunks {
			select {
			case out <- &storer.BinC{Address: c.Address, BatchID: c.BatchID, BinID: c.BinID, StampHash: c.StampHash}:
			case <-ctx.Done():
				select {
				case errC <- ctx.Err():
				default:
				}
			}
		}
	}()

	return out, func() {}, errC
}

func (s *ReserveStore) ReserveSize() int {
	return s.reservesize
}

func (s *ReserveStore) ReserveLastBinIDs() (curs []uint64, epoch uint64, err error) {
	return s.cursors, s.epoch, s.cursorsErr
}

// PutCalls returns the amount of times Put was called.
func (s *ReserveStore) PutCalls() int {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.putCalls
}

// SetCalls returns the amount of times Set was called.
func (s *ReserveStore) SetCalls() int {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.setCalls
}

// Get chunks.
func (s *ReserveStore) ReserveGet(ctx context.Context, addr swarm.Address, batchID []byte, stampHash []byte) (swarm.Chunk, error) {
	if s.evilAddr.Equal(addr) {
		// inject the malicious chunk instead
		return s.evilChunk, nil
	}

	if v, ok := s.chunks[addr.String()+string(batchID)+string(stampHash)]; ok {
		return v, nil
	}

	return nil, storage.ErrNotFound
}

// Put chunks.
func (s *ReserveStore) ReservePutter() storage.Putter {
	return storage.PutterFunc(
		func(ctx context.Context, c swarm.Chunk) error {
			s.mtx.Lock()
			s.putCalls++
			s.mtx.Unlock()
			return s.put(ctx, c)
		},
	)
}

// Put chunks.
func (s *ReserveStore) put(_ context.Context, chs ...swarm.Chunk) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	for _, c := range chs {
		if s.putHook != nil {
			if err := s.putHook(c); err != nil {
				return err
			}
		}
		stampHash, err := c.Stamp().Hash()
		if err != nil {
			return err
		}
		s.chunks[c.Address().String()+string(c.Stamp().BatchID())+string(stampHash)] = c
	}
	return nil
}

// Has chunks.
func (s *ReserveStore) ReserveHas(addr swarm.Address, batchID []byte, stampHash []byte) (bool, error) {
	if _, ok := s.chunks[addr.String()+string(batchID)+string(stampHash)]; !ok {
		return false, nil
	}
	return true, nil
}

func (s *ReserveStore) ReserveSample(context.Context, []byte, uint8, uint64, *big.Int) (storer.Sample, error) {
	return s.sample, nil
}

type Option interface {
	apply(*ReserveStore)
}
type optionFunc func(*ReserveStore)

func (f optionFunc) apply(r *ReserveStore) { f(r) }
