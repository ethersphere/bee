// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullstorage

import (
	"context"
	"errors"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	_ Storer = (*ps)(nil)
	// ErrDbClosed is used to signal the underlying database was closed
	ErrDbClosed = errors.New("db closed")
)

// Storer is a thin wrapper around storage.Storer.
// It is used in order to collect and provide information about chunks
// currently present in the local store.
type Storer interface {
	// IntervalChunks collects chunk for a requested interval.
	IntervalChunks(ctx context.Context, bin uint8, from, to uint64, limit int) (chunks []swarm.Address, topmost uint64, err error)
	// Get chunks.
	Get(ctx context.Context, mode storage.ModeGet, addrs ...swarm.Address) ([]swarm.Chunk, error)
	// Put chunks.
	Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) error
	// Set chunks.
	Set(ctx context.Context, mode storage.ModeSet, addrs ...swarm.Address) error
	// Has chunks.
	Has(ctx context.Context, addr swarm.Address) (bool, error)
}

// ps wraps storage.Storer.
type ps struct {
	storage.Storer
}

// New returns a new pullstorage Storer instance.
func New(storer storage.Storer) Storer {
	return &ps{storer}
}

// IntervalChunks collects chunk for a requested interval.
func (s *ps) IntervalChunks(ctx context.Context, bin uint8, from, to uint64, limit int) (chs []swarm.Address, topmost uint64, err error) {
	// call iterator, iterate either until upper bound or limit reached
	// return addresses, topmost is the topmost bin ID

	ch, dbClosed, stop := s.SubscribePull(ctx, bin, from, to)
	defer stop()

	var nomore bool

LOOP:
	for limit > 0 {
		select {
		case v, ok := <-ch:
			if !ok {
				nomore = true
				break LOOP
			}
			chs = append(chs, v.Address)
			if v.BinID > topmost {
				topmost = v.BinID
			}
			limit--
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		}
	}

	select {
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	case <-dbClosed:
		return nil, 0, ErrDbClosed
	default:
	}

	if nomore {
		// end of interval reached. no more chunks so interval is complete
		// return requested `to`
		topmost = to
	}

	return chs, topmost, nil
}

// Get chunks.
func (s *ps) Get(ctx context.Context, mode storage.ModeGet, addrs ...swarm.Address) ([]swarm.Chunk, error) {
	return s.Storer.GetMulti(ctx, mode, addrs...)
}

// Put chunks.
func (s *ps) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) error {
	_, err := s.Storer.Put(ctx, mode, chs...)
	return err
}
