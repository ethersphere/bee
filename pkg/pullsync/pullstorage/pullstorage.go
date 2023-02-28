// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullstorage

import (
	"context"
	"errors"
	"fmt"
	"time"

	storer "github.com/ethersphere/bee/pkg/localstorev2"
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/swarm"
	"resenje.org/singleflight"
)

const loggerName = "pullstorage"

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
	Has(addr swarm.Address, binID uint64) (bool, error)
	Put(ctx context.Context, chunks ...swarm.Chunk) error
	Get(ctx context.Context, addr swarm.Address, binID uint64) (swarm.Chunk, error)
}

// PullStorer wraps storage.Storer.
type PullStorer struct {
	store       storer.ReserveStore
	intervalsSF singleflight.Group
	logger      log.Logger
	metrics     metrics
}

// New returns a new pullstorage Storer instance.
func New(store storer.ReserveStore, logger log.Logger) *PullStorer {
	return &PullStorer{
		store:   store,
		metrics: newMetrics(),
		logger:  logger.WithName(loggerName).Register(),
	}
}

// IntervalChunks collects chunk for a requested interval.
func (s *PullStorer) IntervalChunks(ctx context.Context, bin uint8, from, to uint64, limit int) ([]swarm.Address, uint64, error) {
	loggerV2 := s.logger.V(2).Register()

	type result struct {
		chs     []swarm.Address
		topmost uint64
	}
	s.metrics.TotalSubscribePullRequests.Inc()
	defer s.metrics.TotalSubscribePullRequestsComplete.Inc()

	v, _, err := s.intervalsSF.Do(ctx, fmt.Sprintf("%v-%v-%v-%v", bin, from, to, limit), func(ctx context.Context) (interface{}, error) {
		var (
			chs     []swarm.Address
			topmost uint64
		)
		// call iterator, iterate either until upper bound or limit reached
		// return addresses, topmost is the topmost bin ID
		var (
			timer  *time.Timer
			timerC <-chan time.Time
		)
		s.metrics.SubscribePullsStarted.Inc()
		chC, stop, errC := s.store.SubscribeBin(ctx, bin, from, to)
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
			case c, ok := <-chC:
				if !ok {
					nomore = true
					break LOOP
				}
				chs = append(chs, c.Address)
				if c.BinID > topmost {
					topmost = c.BinID
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
			case err := <-errC:
				return nil, err
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-timerC:
				loggerV2.Debug("batch timeout timer triggered")
				// return batch if new chunks are not received after some time
				break LOOP
			}
		}

		if nomore {
			// end of interval reached. no more chunks so interval is complete
			// return requested `to`. it could be that len(chs) == 0 if the interval
			// is empty
			loggerV2.Debug("no more batches from the subscription", "to", to, "topmost", topmost)
			topmost = to
		}

		return &result{chs: chs, topmost: topmost}, nil
	})

	if err != nil {
		s.metrics.SubscribePullsFailures.Inc()
		return nil, 0, err
	}
	r := v.(*result)
	return r.chs, r.topmost, nil
}

// Cursors gets the last BinID for every bin in the local storage
func (s *PullStorer) Cursors(ctx context.Context) (curs []uint64, err error) {
	return s.store.ReserveLastBinIDs()
}

func (s *PullStorer) Has(addr swarm.Address, batchID [] ) (bool, error) {
	return s.store.ReserveHas(addr, binID)
}

func (s *PullStorer) Put(ctx context.Context, chunks ...swarm.Chunk) error {

	putter := s.store.ReservePutter(ctx)
	for _, c := range chunks {
		err := putter.Put(ctx, c)
		if err != nil {
			putter.Cleanup()
			return err
		}
	}
	return putter.Done(swarm.ZeroAddress)
}

func (s *PullStorer) Get(ctx context.Context, addr swarm.Address, binID uint64) (swarm.Chunk, error) {

	c, err := s.store.ReserveGet(ctx, addr, binID)
	if err != nil {
		return nil, err
	}

	return c, nil
}
