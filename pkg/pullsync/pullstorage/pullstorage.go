// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullstorage

import (
	"context"
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

	// after how long to return a non-empty batch
	batchTimeout = time.Second
)

// Storer is a thin wrapper around storage.Storer.
// It is used in order to collect and provide information about chunks
// currently present in the local store.
type Storer interface {
	// IntervalChunks collects chunk for a requested interval.
	IntervalChunks(ctx context.Context, bin uint8, from, limit uint64) (chunks []*BinC, topmost uint64, err error)
	// Cursors gets the last BinID for every bin in the local storage
	Cursors(ctx context.Context) ([]uint64, error)
	Has(addr swarm.Address, batchID []byte) (bool, error)
	Put(ctx context.Context, chunks ...swarm.Chunk) error
	Get(ctx context.Context, addr swarm.Address, batchID []byte) (swarm.Chunk, error)
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

type BinC struct {
	Address swarm.Address
	BatchID []byte
}

// IntervalChunks collects chunks at a bin starting at some start BinID until a limit is reached.
// The function waits for an unbounded amount of time for the first chunk to arrive.
// After the arrival of the first chunk, the subsequent chunks have a limited amount of time to arrive,
// after which the function returns the collected slice of chunks.
func (s *PullStorer) IntervalChunks(ctx context.Context, bin uint8, start, limit uint64) ([]*BinC, uint64, error) {
	loggerV2 := s.logger.V(2).Register()

	type result struct {
		chs     []*BinC
		topmost uint64
	}
	s.metrics.TotalSubscribePullRequests.Inc()
	defer s.metrics.TotalSubscribePullRequestsComplete.Inc()

	v, _, err := s.intervalsSF.Do(ctx, fmt.Sprintf("%v-%v-%v", bin, start, limit), func(ctx context.Context) (interface{}, error) {
		var (
			chs     []*BinC
			topmost uint64
			timer   *time.Timer
			timerC  <-chan time.Time
		)
		s.metrics.SubscribePullsStarted.Inc()
		chC, unsub, errC := s.store.SubscribeBin(ctx, bin, start)
		defer func() {
			unsub()
			if timer != nil {
				timer.Stop()
			}
			s.metrics.SubscribePullsComplete.Inc()
		}()

	LOOP:
		for limit > 0 {
			select {
			case c := <-chC:
				chs = append(chs, &BinC{Address: c.Address, BatchID: c.BatchID})
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

func (s *PullStorer) Has(addr swarm.Address, batchID []byte) (bool, error) {
	return s.store.ReserveHas(addr, batchID)
}

func (s *PullStorer) Put(ctx context.Context, chunks ...swarm.Chunk) error {
	putter := s.store.ReservePutter(ctx)
	for _, c := range chunks {
		err := putter.Put(ctx, c)
		if err != nil {
			_ = putter.Cleanup()
			return err
		}
	}
	return putter.Done(swarm.ZeroAddress)
}

func (s *PullStorer) Get(ctx context.Context, addr swarm.Address, batchID []byte) (swarm.Chunk, error) {
	c, err := s.store.ReserveGet(ctx, addr, batchID)
	if err != nil {
		return nil, err
	}

	return c, nil
}
