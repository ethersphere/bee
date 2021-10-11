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
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/transaction"
	lru "github.com/hashicorp/golang-lru"
)

const (
	defaultFlagTimeout = 5 * time.Minute
	defaultPollEvery   = 1 * time.Minute
	cleanupTimeout     = 5 * time.Minute // how long before we cleanup a peer
	blocksToRemember   = 1000
)

// zeroTime exists to avoid unnecessary allocations.
var zeroTime = time.Time{}

type prover interface {
	Prove(context.Context, swarm.Address, uint64) ([]byte, error)
}

type peer struct {
	lastSeen     time.Time // last time we've seen the peer (used to gc entries)
	flaggedSince time.Time // timestamp of the point we've timed-out or got an error from a peer
}

func (p *peer) flag() {
	if p.flaggedSince.IsZero() {
		p.flaggedSince = time.Now()
	}
}
func (p *peer) unflag() {
	p.flaggedSince = zeroTime
}
func (p *peer) unsynced(now time.Time, flagTimeout time.Duration) bool {
	return !p.flaggedSince.IsZero() && now.After(p.flaggedSince.Add(flagTimeout))
}

type Options struct {
	FlagTimeout time.Duration
	PollEvery   time.Duration
}

type ChainSyncer struct {
	backend                transaction.Backend // eth backend
	prove                  prover              // the chainsync protocol
	peerIterator           topology.EachPeerer // topology peer iterator
	peers                  *sync.Map           // list of peers and their metadata
	pollEvery, flagTimeout time.Duration
	logger                 logging.Logger
	lru                    *lru.Cache
	metrics                metrics

	quit chan struct{}
	wg   sync.WaitGroup
}

func New(backend transaction.Backend, p prover, peerIterator topology.EachPeerer, logger logging.Logger, o *Options) (*ChainSyncer, error) {
	lruCache, err := lru.New(blocksToRemember)
	if err != nil {
		return nil, err
	}
	if o == nil {
		o = &Options{
			FlagTimeout: defaultFlagTimeout,
			PollEvery:   defaultPollEvery,
		}
	}
	c := &ChainSyncer{
		backend:      backend,
		prove:        p,
		peerIterator: peerIterator,
		pollEvery:    o.PollEvery,
		flagTimeout:  o.FlagTimeout,
		logger:       logger,
		peers:        &sync.Map{},
		lru:          lruCache,
		metrics:      newMetrics(),
		quit:         make(chan struct{}),
	}
	c.wg.Add(1)
	go c.manage()
	return c, nil
}

func (c *ChainSyncer) manage() {
	defer c.wg.Done()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-c.quit
		cancel()
	}()
	var wg sync.WaitGroup
	for {
		select {
		case <-c.quit:
			return
		case <-time.After(c.pollEvery):
		}
		// go through every peer we are connected to
		// try to ask about a recent block height.
		// if they answer, we unflag them
		// if not, they get flagged with time.Now() (in case they weren't
		// flagged before).
		// once subsequent checks continue failing we eventually
		// kick them away.
		blockHeight, blockHash, err := c.getBlockHeight(ctx)
		if err != nil {
			c.logger.Warningf("chainsyncer: failed getting block height for challenge: %v", err)
			continue
		}

		start := time.Now()

		_ = c.peerIterator.EachPeer(func(p swarm.Address, _ uint8) (bool, bool, error) {
			entry, _ := c.peers.LoadOrStore(p.ByteString(), &peer{})
			e := entry.(*peer)
			e.lastSeen = time.Now()
			wg.Add(1)
			go func() {
				defer wg.Done()
				hash, err := c.prove.Prove(ctx, p, blockHeight)
				if err != nil {
					c.logger.Infof("chainsync: peer %s failed to prove block %d: %v", p.String(), blockHeight, err)
					c.metrics.PeerErrors.Inc()
					e.flag()
					return
				}
				if !bytes.Equal(blockHash, hash) {
					c.logger.Infof("chainsync: peer %s failed to prove block %d: want block hash %x got %x", p.String(), blockHash, hash)
					c.metrics.InvalidProofs.Inc()
					e.flag()
					return
				}
				c.logger.Tracef("chainsync: peer %s proved block %d", p.String(), blockHeight)
				c.metrics.SyncedPeers.Inc()
				e.unflag()
			}()
			return false, false, nil
		}, topology.Filter{})

		// wait for all operations to finish
		wg.Wait()

		select {
		case <-c.quit:
			return
		default:
		}

		c.metrics.TotalTimeWaiting.Add(float64(time.Since(start)))

		c.cleanup()
		c.block()
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

// block unsynced peers
func (c *ChainSyncer) block() {
	now := time.Now()
	var keys []string
	c.peers.Range(func(k, v interface{}) bool {
		peer := v.(*peer)
		if peer.unsynced(now, c.flagTimeout) {
			// peer designated for blocking
			c.logger.Infof("chainsync: peer %s unsynced, flagged since %s", swarm.NewAddress([]byte(k.(string))).String(), peer.flaggedSince)
			c.metrics.UnsyncedPeers.Inc()
			keys = append(keys, k.(string))
		}
		return true
	})

	for range keys {
		if notifyHook != nil {
			notifyHook()
		}
	}
}

// cleanup old peers
func (c *ChainSyncer) cleanup() {
	now := time.Now()
	var keys []string
	c.peers.Range(func(k, v interface{}) bool {
		peer := v.(*peer)
		if now.After(peer.lastSeen.Add(cleanupTimeout)) {
			keys = append(keys, k.(string))
		}
		return true
	})

	for _, k := range keys {
		c.peers.Delete(k)
	}
}

func (c *ChainSyncer) Close() error {
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	close(c.quit)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		return errors.New("chainsyncer shutting down with running goroutines")
	}
	return nil
}

var notifyHook func()
