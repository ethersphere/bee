// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"context"
	"sync"

	"github.com/ethersphere/bee/pkg/pullsync/pullstorage"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var _ pullstorage.Storer = (*PullStorage)(nil)

type chunksResponse struct {
	chunks  []swarm.Address
	topmost uint64
	err     error
}

// WithIntervalsResp mocks a desired response when calling IntervalChunks method.
// Different possible responses for subsequent responses in multi-call scenarios
// are possible (i.e. first call yields a,b,c, second call yields d,e,f).
// Mock maintains state of current call using chunksCalls counter.
func WithIntervalsResp(addrs []swarm.Address, top uint64, err error) Option {
	return optionFunc(func(p *PullStorage) {
		p.intervalChunksResponses = append(p.intervalChunksResponses, chunksResponse{chunks: addrs, topmost: top, err: err})
	})
}

// WithChunks mocks the set of chunks that the store is aware of (used in Get and Has calls).
func WithChunks(chs ...swarm.Chunk) Option {
	return optionFunc(func(p *PullStorage) {
		for _, c := range chs {
			p.chunks[c.Address().String()] = c.Data()
		}
	})
}

// WithEvilChunk allows to inject a malicious chunk (request a certain address
// of a chunk, but get another), in order to mock unsolicited chunk delivery.
func WithEvilChunk(addr swarm.Address, ch swarm.Chunk) Option {
	return optionFunc(func(p *PullStorage) {
		p.evilAddr = addr
		p.evilChunk = ch
	})

}

type PullStorage struct {
	mtx         sync.Mutex
	chunksCalls int
	putCalls    int

	chunks    map[string][]byte
	evilAddr  swarm.Address
	evilChunk swarm.Chunk

	intervalChunksResponses []chunksResponse
}

// NewPullStorage returns a new PullStorage mock.
func NewPullStorage(opts ...Option) *PullStorage {
	s := &PullStorage{
		chunks: make(map[string][]byte),
	}
	for _, v := range opts {
		v.apply(s)
	}
	return s
}

// IntervalChunks returns a set of chunk in a requested interval.
func (s *PullStorage) IntervalChunks(_ context.Context, bin uint8, from, to uint64, limit int) (chunks []swarm.Address, topmost uint64, err error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	r := s.intervalChunksResponses[s.chunksCalls]
	s.chunksCalls++

	return r.chunks, r.topmost, r.err
}

// PutCalls returns the amount of times Put was called.
func (s *PullStorage) PutCalls() int {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	return s.putCalls
}

// Get chunks.
func (s *PullStorage) Get(_ context.Context, _ storage.ModeGet, addrs ...swarm.Address) (chs []swarm.Chunk, err error) {
	for _, a := range addrs {
		if s.evilAddr.Equal(a) {
			//inject the malicious chunk instead
			chs = append(chs, s.evilChunk)
			continue
		}

		if v, ok := s.chunks[a.String()]; ok {
			chs = append(chs, swarm.NewChunk(a, v))
		} else if !ok {
			return nil, storage.ErrNotFound
		}
	}
	return chs, nil
}

// Put chunks.
func (s *PullStorage) Put(_ context.Context, _ storage.ModePut, chs ...swarm.Chunk) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	for _, c := range chs {
		s.chunks[c.Address().String()] = c.Data()
	}
	s.putCalls++
	return nil
}

// Set chunks.
func (s *PullStorage) Set(ctx context.Context, mode storage.ModeSet, addrs ...swarm.Address) error {
	return nil
}

// Has chunks.
func (s *PullStorage) Has(_ context.Context, addr swarm.Address) (bool, error) {
	if _, ok := s.chunks[addr.String()]; !ok {
		return false, nil
	}
	return true, nil
}

type Option interface {
	apply(*PullStorage)
}
type optionFunc func(*PullStorage)

func (f optionFunc) apply(r *PullStorage) { f(r) }
