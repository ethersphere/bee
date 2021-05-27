// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullstorage

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sync/singleflight"
	"io"
	"time"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	_ Storer = (*PullStorer)(nil)
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

	io.Closer
}

// PullStorer wraps storage.Storer.
type PullStorer struct {
	storage.Storer
	intervalFlight singleflight.Group
	metrics        metrics
	psCtx          context.Context
	psCancel       context.CancelFunc
}

// New returns a new pullstorage Storer instance.
func New(storer storage.Storer) *PullStorer {
	ctx, cancel := context.WithCancel(context.Background())
	return &PullStorer{
		Storer:   storer,
		metrics:  newMetrics(),
		psCtx:    ctx,
		psCancel: cancel,
	}
}

type intervalFlightResult struct {
	chs     []swarm.Address
	topmost uint64
}

// IntervalChunks collects chunk for a requested interval.
func (s *PullStorer) IntervalChunks(ctx context.Context, bin uint8, from, to uint64, limit int) ([]swarm.Address, uint64, error) {
	s.metrics.TotalSubscribePullRequests.Inc()
	defer s.metrics.TotalSubscribePullRequestsComplete.Inc()
	resultC := s.intervalFlight.DoChan(subKey(bin, from, to, limit), func() (interface{}, error) {
		// call iterator, iterate either until upper bound or limit reached
		// return addresses, topmost is the topmost bin ID
		var (
			timer   *time.Timer
			timerC  <-chan time.Time
			chs     []swarm.Address
			topmost uint64
		)
		s.metrics.SubscribePullsStarted.Inc()
		ch, dbClosed, stop := s.SubscribePull(s.psCtx, bin, from, to)
		defer func(start time.Time) {
			stop()
			if timer != nil {
				timer.Stop()
			}
			s.metrics.SubscribePullsComplete.Inc()
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
			case <-timerC:
				// return batch if new chunks are not received after some time
				break LOOP
			case <-s.psCtx.Done():
				// this is called on module shutdown, close all listeners
				return nil, ctx.Err()
			}
		}

		select {
		case <-dbClosed:
			return nil, ErrDbClosed
		default:

		}

		if nomore {
			// end of interval reached. no more chunks so interval is complete
			// return requested `to`. it could be that len(chs) == 0 if the interval
			// is empty
			topmost = to
		}

		return &intervalFlightResult{chs: chs, topmost: topmost}, nil
	})
	select {
	case <-ctx.Done():
	case r := <-resultC:
		if r.Err != nil {
			s.metrics.SubscribePullsFailures.Inc()
			return nil, 0, r.Err
		}
		intervalRes := r.Val.(*intervalFlightResult)
		return intervalRes.chs, intervalRes.topmost, nil
	}
	return nil, 0, ctx.Err()
}

func subKey(bin uint8, from, to uint64, limit int) string {
	return fmt.Sprintf("%d_%d_%d_%d", bin, from, to, limit)
}

// Cursors gets the last BinID for every bin in the local storage
func (s *PullStorer) Cursors(ctx context.Context) (curs []uint64, err error) {
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
func (s *PullStorer) Get(ctx context.Context, mode storage.ModeGet, addrs ...swarm.Address) ([]swarm.Chunk, error) {
	return s.Storer.GetMulti(ctx, mode, addrs...)
}

// Put chunks.
func (s *PullStorer) Put(ctx context.Context, mode storage.ModePut, chs ...swarm.Chunk) error {
	_, err := s.Storer.Put(ctx, mode, chs...)
	return err
}

func (s *PullStorer) Close() error {
	s.psCancel()
	return nil
}
