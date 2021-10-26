// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package chainsyncer provides orchestration logic for
// the chainsync protocol.
package chainsyncer

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethersphere/bee/pkg/blocker"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/transaction"
	lru "github.com/hashicorp/golang-lru"
)

const (
	defaultFlagTimeout     = 10 * time.Minute
	defaultPollEvery       = 1 * time.Minute
	defaultBlockerPollTime = 10 * time.Second
	blockDuration          = 24 * time.Hour

	blocksToRemember = 1000
)

type prover interface {
	Prove(context.Context, swarm.Address, uint64) ([]byte, error)
}

type Options struct {
	FlagTimeout     time.Duration
	PollEvery       time.Duration
	BlockerPollTime time.Duration
}

type ChainSyncer struct {
	backend                transaction.Backend // eth backend
	prove                  prover              // the chainsync protocol
	peerIterator           topology.EachPeerer // topology peer iterator
	pollEvery, flagTimeout time.Duration
	logger                 logging.Logger
	lru                    *lru.Cache
	blocker                *blocker.Blocker
	metrics                metrics

	quit chan struct{}
	wg   sync.WaitGroup
}

func New(backend transaction.Backend, p prover, peerIterator topology.EachPeerer, disconnecter p2p.Disconnecter, logger logging.Logger, o *Options) (*ChainSyncer, error) {
	lruCache, err := lru.New(blocksToRemember)
	if err != nil {
		return nil, err
	}
	if o == nil {
		o = &Options{
			FlagTimeout:     defaultFlagTimeout,
			PollEvery:       defaultPollEvery,
			BlockerPollTime: defaultBlockerPollTime,
		}
	}

	c := &ChainSyncer{
		backend:      backend,
		prove:        p,
		peerIterator: peerIterator,
		pollEvery:    o.PollEvery,
		flagTimeout:  o.FlagTimeout,
		logger:       logger,
		lru:          lruCache,
		metrics:      newMetrics(),
		quit:         make(chan struct{}),
	}

	cb := func(a swarm.Address) {
		c.logger.Warningf("chainsyncer: peer %s is unsynced and will be temporarily blocklisted", a.String())
		c.metrics.UnsyncedPeers.Inc()
	}
	c.blocker = blocker.New(disconnecter, o.FlagTimeout, blockDuration, o.BlockerPollTime, cb, logger)

	c.wg.Add(1)
	go c.manage()
	return c, nil
}

func (c *ChainSyncer) manage() {
	defer c.wg.Done()
	var (
		ctx, cancel = context.WithCancel(context.Background())
		wg          sync.WaitGroup
		o           sync.Once
		positives   int32
		items       int
		ticker      = time.NewTicker(1 * time.Nanosecond)
	)
	go func() {
		<-c.quit
		cancel()
		ticker.Stop()
	}()

	for {
		select {
		case <-c.quit:
			return
		case <-ticker.C:
			o.Do(func() {
				ticker.Reset(c.pollEvery)
			})
		}
		// go through every peer we are connected to
		// try to ask about a recent block height.
		// if they answer, we unflag them
		// if not, they get flagged with time.Now() (in case they weren't
		// flagged before).
		// when subsequent checks continue failing we eventually blocklist.
		blockHeight, blockHash, err := c.getBlockHeight(ctx)
		if err != nil {
			c.logger.Warningf("chainsyncer: failed getting block height for challenge: %v", err)
			continue
		}

		start := time.Now()
		items = 0
		positives = 0
		_ = c.peerIterator.EachPeer(func(p swarm.Address, _ uint8) (bool, bool, error) {
			wg.Add(1)
			items++
			go func(p swarm.Address) {
				defer wg.Done()
				hash, err := c.prove.Prove(ctx, p, blockHeight)
				if err != nil {
					c.logger.Infof("chainsync: peer %s failed to prove block %d: %v", p.String(), blockHeight, err)
					c.metrics.PeerErrors.Inc()
					c.blocker.Flag(p)
					return
				}
				if !bytes.Equal(blockHash, hash) {
					c.logger.Infof("chainsync: peer %s failed to prove block %d: want block hash %x got %x", p.String(), blockHeight, blockHash, hash)
					c.metrics.InvalidProofs.Inc()
					c.blocker.Flag(p)
					return
				}
				c.logger.Tracef("chainsync: peer %s proved block %d", p.String(), blockHeight)
				c.metrics.SyncedPeers.Inc()
				c.blocker.Unflag(p)
				atomic.AddInt32(&positives, 1)
			}(p)
			return false, false, nil
		}, topology.Filter{Reachable: true})

		// wait for all operations to finish
		wg.Wait()

		c.metrics.PositiveProofs.Set(float64(positives) / float64(items))

		select {
		case <-c.quit:
			return
		default:
		}

		c.metrics.TotalTimeWaiting.Add(float64(time.Since(start)))
	}
}

// getBlockHeight returns a block height to challenge peers
// with. it does not return the highest block height due to
// concerns of block propagation time that could create a
// situation where purely functional nodes are challenged with
// block heights they still did not see. we therefore pick
// a block height which is a few minutes old, so that relative
// synchronization is maintained.
func (c *ChainSyncer) getBlockHeight(ctx context.Context) (uint64, []byte, error) {
	current, err := c.backend.BlockNumber(ctx)
	if err != nil {
		return 0, nil, err
	}
	if current < 60 {
		// needed for testing environments where a fresh
		// blockchain is used.
		current = 1
	} else {
		// shave ~one minute off
		current -= 10

		// now quanitize so that we limit the number of distinct blocks
		// we ask the peers, thereby minimizing the amount of backend
		// calls they need to do (this should be about 64 blocks quantas)
		current = (current >> 6) << 6
	}

	var blockHash []byte
	if val, ok := c.lru.Get(current); ok {
		blockHash = val.([]byte)
	} else {
		height := big.NewInt(0).SetUint64(current)
		hdr, err := c.backend.HeaderByNumber(ctx, height)
		if err != nil {
			return 0, nil, err
		}
		_ = c.lru.Add(current, hdr.Hash().Bytes())
		blockHash = hdr.Hash().Bytes()
	}
	return current, blockHash, nil
}

func (c *ChainSyncer) Close() error {
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	close(c.quit)
	_ = c.blocker.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		return errors.New("chainsyncer shutting down with running goroutines")
	}
	return nil
}
