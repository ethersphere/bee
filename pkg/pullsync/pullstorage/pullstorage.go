// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullstorage

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	_ Storer = (*ps)(nil)
	// ErrDbClosed is used to signal the underlying database was closed
	ErrDbClosed = errors.New("db closed")

	// after how long to return a non-empty batch
	batchTimeout = 500 * time.Millisecond
)

// Storer is a thin wrapper around storage.Storer.
// It is used in order to collect and provide information about chunks
// currently present in the local store.
type Storer interface {
	// IntervalChunks collects chunk for a requested interval.
	IntervalChunks(ctx context.Context, bin uint8, from, to uint64, limit int) (chunks []swarm.Address, topmost uint64, err error)
	// Cursors gets the last BinID for every bin in the local storage
	Cursors(ctx context.Context) ([]uint64, error)
	// Get chunks.
	Get(ctx context.Context, mode storage.ModeGet, addrs ...swarm.Address) ([]swarm.Chunk, error)
	// Put chunks.
	Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) error
	// Set chunks.
	Set(ctx context.Context, mode storage.ModeSet, addrs ...swarm.Address) error
	// Has chunks.
	Has(ctx context.Context, addr swarm.Address) (bool, error)
}

type intervalChunks struct {
	chs     []swarm.Address
	topmost uint64
	err     error
}

// ps wraps storage.Storer.
type ps struct {
	storage.Storer

	openSubs   map[string][]chan intervalChunks
	openSubsMu sync.Mutex
}

// New returns a new pullstorage Storer instance.
func New(storer storage.Storer) Storer {
	return &ps{
		Storer:   storer,
		openSubs: make(map[string][]chan intervalChunks),
	}
}

// IntervalChunks collects chunk for a requested interval.
func (s *ps) IntervalChunks(ctx context.Context, bin uint8, from, to uint64, limit int) (chs []swarm.Address, topmost uint64, err error) {
	var (
		k = subKey(bin, from, to, limit)
		c = make(chan intervalChunks)
	)
	s.openSubsMu.Lock()
	if subs, ok := s.openSubs[k]; ok {
		// some subscription already exists, add ours
		// and wait for the result
		subs = append(subs, c)
		s.openSubs[k] = subs
		s.openSubsMu.Unlock()
		select {
		case res := <-c:
			// since this is a simple read from a channel, one slow
			// peer requesting chunks will not affect another
			return res.chs, res.topmost, res.err
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		}
	}
	s.openSubs[k] = make([]chan intervalChunks, 0)
	s.openSubsMu.Unlock()

	// call iterator, iterate either until upper bound or limit reached
	// return addresses, topmost is the topmost bin ID
	var (
		timer  *time.Timer
		timerC <-chan time.Time
	)

	ch, dbClosed, stop := s.SubscribePull(ctx, bin, from, to)
	defer func(start time.Time) {
		stop()
		if timer != nil {
			timer.Stop()
		}

		// tell others about the results
		s.openSubsMu.Lock()
		for _, c := range s.openSubs[k] {
			c <- intervalChunks{chs: chs, topmost: topmost, err: err}
		}
		delete(s.openSubs, k)
		s.openSubsMu.Unlock()

	}(time.Now())

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
			if timer == nil {
				timer = time.NewTimer(batchTimeout)
			} else {
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(batchTimeout)
			}
			timerC = timer.C
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case <-timerC:
			// return batch if new chunks are not received after some time
			break LOOP
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
		// return requested `to`. it could be that len(chs) == 0 if the interval
		// is empty
		topmost = to
	}

	return chs, topmost, nil
}

// Cursors gets the last BinID for every bin in the local storage
func (s *ps) Cursors(ctx context.Context) (curs []uint64, err error) {
	curs = make([]uint64, swarm.MaxBins)
	for i := uint8(0); i < swarm.MaxBins; i++ {
		binID, err := s.Storer.LastPullSubscriptionBinID(i)
		if err != nil {
			return nil, err
		}
		curs[i] = binID
	}
	return curs, nil
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

func subKey(bin uint8, from, to uint64, limit int) string {
	return fmt.Sprintf("%d_%d_%d_%d", bin, from, to, limit)
}
