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
	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/transaction"
	lru "github.com/hashicorp/golang-lru"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "chainsyncer"

const (
	defaultFlagTimeout     = 15 * time.Minute
	defaultPollEvery       = 5 * time.Minute
	defaultBlockerPollTime = 30 * time.Second
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
	backend                transaction.Backend   // eth backend
	prove                  prover                // the chainsync protocol
	peerIterator           topology.PeerIterator // topology peer iterator
	pollEvery, flagTimeout time.Duration
	logger                 log.Logger
	lru                    *lru.Cache
	blocker                *blocker.Blocker
	disconnecter           p2p.Disconnecter
	metrics                metrics

	quit chan struct{}
	wg   sync.WaitGroup
}

func New(backend transaction.Backend, p prover, peerIterator topology.PeerIterator, disconnecter p2p.Disconnecter, logger log.Logger, o *Options) (*ChainSyncer, error) {
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
		logger:       logger.WithName(loggerName).Register(),
		lru:          lruCache,
		disconnecter: disconnecter,
		metrics:      newMetrics(),
		quit:         make(chan struct{}),
	}

	cb := func(a swarm.Address) {
		c.logger.Warning("peer is unsynced and will be temporarily blocklisted", "peer_address", a)
		c.metrics.UnsyncedPeers.Inc()
	}
	c.blocker = blocker.New(disconnecter, o.FlagTimeout, blockDuration, o.BlockerPollTime, cb, c.logger)

	c.wg.Add(1)
	go c.manage()
	return c, nil
}

func (c *ChainSyncer) manage() {
	loggerV2 := c.logger.V(2).Register()

	defer c.wg.Done()
	var (
		ctx, cancel = context.WithCancel(context.Background())
		wg          sync.WaitGroup
		positives   int32
		items       int
		ticker      = time.NewTicker(c.pollEvery)
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
		}
		// go through every peer we are connected to
		// try to ask about a recent block height.
		// if they answer, we unflag them
		// if not, they get flagged with time.Now() (in case they weren't
		// flagged before).
		// when subsequent checks continue failing we eventually blocklist.
		blockHeight, blockHash, err := c.getBlockHeight(ctx)
		if err != nil {
			c.logger.Warning("failed getting block height for challenge", "error", err)
			continue
		}

		start := time.Now()
		items = 0
		positives = 0
		var seen []swarm.Address
		cctx, cancelF := context.WithTimeout(ctx, defaultPollEvery)
		_ = c.peerIterator.EachConnectedPeer(func(p swarm.Address, _ uint8) (bool, bool, error) {
			seen = append(seen, p)
			wg.Add(1)
			items++
			go func(peer swarm.Address) {
				defer wg.Done()
				hash, err := c.prove.Prove(cctx, peer, blockHeight)
				if err != nil {
					c.logger.Debug("failed to prove block", "peer", peer, "block", blockHeight, "elapsed", time.Since(start), "error", err)
					c.metrics.PeerErrors.Inc()
					c.blocker.Flag(peer)
					return
				}
				if !bytes.Equal(blockHash, hash) {
					c.logger.Debug("failed to prove block", "peer", peer, "block", blockHeight, "elapsed", time.Since(start), "want hash", blockHash, "have hash", hash)
					c.metrics.InvalidProofs.Inc()
					c.blocker.Flag(peer)
					return
				}
				loggerV2.Debug("block successfully proved", "peer", peer, "block", blockHeight, "elapsed", time.Since(start))
				c.metrics.SyncedPeers.Inc()
				c.blocker.Unflag(peer)
				atomic.AddInt32(&positives, 1)
			}(p)
			return false, false, nil
		}, topology.Select{Reachable: true})

		// wait for all operations to finish
		wg.Wait()

		cancelF()

		// prune the peers that we haven't seen.
		c.blocker.PruneUnseen(seen)

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
