// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package chainsyncer provides orchestration logic for
// the chainsync protocol.
package chainsyncer

import (
	"bytes"
	"context"
	"math/big"
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	"github.com/ethersphere/bee/pkg/transaction"
)

var (
	flagTimeout    = 5 * time.Minute // how long before blocking a flagged peer
	cleanupTimeout = 5 * time.Minute // how long before we cleanup a peer
	blockDuration  = 24 * time.Hour  // how long to blocklist an unresponsive peer for
)

type prover interface {
	Prove(context.Context, swarm.Address, uint64) ([]byte, error)
}

type peer struct {
	lastSeen     time.Time // last time we've seen the peer (used to gc entries)
	lastCheck    time.Time // last time we've sent a request to prove a block
	flagged      bool      // indicates whether the peer is actively flagged
	flaggedSince time.Time // timestamp of the point we've timed-out or got an error from a peer
}

func (p *peer) flag() {
	if !p.flagged {
		p.flaggedSince = time.Now()
		p.flagged = true
	}
}
func (p *peer) unflag() {
	p.flagged = false
}
func (p *peer) unsynced(now time.Time) bool {
	return p.flagged && now.After(p.flaggedSince.Add(flagTimeout))
}

type C struct {
	backend      transaction.Backend // eth backend
	prove        prover              // the chainsync protocol
	disconnecter p2p.Disconnecter    // p2p interface to block peers
	peerIterator topology.EachPeerer // topology peer iterator
	peers        *sync.Map           // list of peers and their metadata
	pollEvery    time.Duration
	logger       logging.Logger

	quit chan struct{}
}

func New(backend transaction.Backend, p prover, d p2p.Disconnecter, peerIterator topology.EachPeerer, pollEvery *time.Duration, logger logging.Logger) *C {
	c := &C{
		backend:      backend,
		prove:        p,
		disconnecter: d,
		peerIterator: peerIterator,
		pollEvery:    *pollEvery,
		logger:       logger,
		peers:        &sync.Map{},
		quit:         make(chan struct{}),
	}
	go c.manage()
	return c
}

func (c *C) manage() {
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-c.quit
		cancel()
	}()
	for {
		select {
		case <-c.quit:
			return
		case <-time.After(time.Second * 30): // sleep for a little bit
		}
		// go through every peer we are connected to
		// try to ask about a recent block height.
		// if they answer, we unflag them
		// if not, they get flagged with time.Now() (in case they werent
		// flagged before).
		// once subsequent checks continue failing we eventually
		// kick them away.
		blockHeight, blockHash, err := c.getBlockHeight(ctx)
		if err != nil {
			continue
		}

		_ = c.peerIterator.EachPeer(func(p swarm.Address, _ uint8) (bool, bool, error) {
			var e *peer
			entry, ok := c.peers.Load(p.ByteString())
			if !ok {
				e = &peer{}
			} else {
				e = entry.(*peer)
			}
			e.lastSeen = time.Now()
			wg.Add(1)
			go func() {
				defer wg.Done()
				hash, err := c.prove.Prove(ctx, p, blockHeight)
				if err != nil {
					c.logger.Infof("chainsync: peer %s failed to prove block %d: %v", p.String(), blockHeight, err)
					e.flag()
					return
				}
				if !bytes.Equal(blockHash, hash) {
					c.logger.Infof("chainsync: peer %s failed to prove block %d: want block hash %x got %x", p.String(), blockHash, hash)
					e.flag()
					return
				}
				e.unflag() // if all good then clear possible previous flagging
			}()
			return false, false, nil
		})

		// wait for all operations to finish
		wg.Wait()
		select {
		case <-c.quit:
			return
		default:
		}

		//cleanup old unseen peers
		c.cleanup()
		// block unsynced peers
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
func (c *C) getBlockHeight(ctx context.Context) (uint64, []byte, error) {
	current, err := c.backend.BlockNumber(ctx)
	if err != nil {
		return 0, nil, err
	}
	if current < 60 {
		// needed for testing environments where a fresh
		// blockchain is used.
		current = 1
	} else {
		// on mainnet the blocktime is 5 seconds, so to get 5
		// minutes we need to subtract 60 block heights
		current -= 60
	}
	height := big.NewInt(0).SetUint64(current)
	hdr, err := c.backend.HeaderByNumber(ctx, height)
	if err != nil {
		return 0, nil, err
	}
	return current, hdr.Hash().Bytes(), nil
}

// block unsynced peers
func (c *C) block() {
	now := time.Now()
	var keys []string
	c.peers.Range(func(k, v interface{}) bool {
		peer := v.(*peer)
		if peer.unsynced(now) {
			// peer designated for blocking
			c.logger.Infof("chainsync: blocking peer %s, flagged since %s", swarm.NewAddress([]byte(k.(string))).String(), peer.flaggedSince)
			keys = append(keys, k.(string))
		}
		return true
	})

	for _, k := range keys {
		addr := swarm.NewAddress([]byte(k))
		if err := c.disconnecter.Blocklist(addr, blockDuration); err != nil {
			c.logger.Warningf("chainsync: blocking peer %s failed: %v", addr, err)
		}
	}
}

// cleanup old peers
func (c *C) cleanup() {
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

func (c *C) Close() error {
	//todo wait on goroutines
	close(c.quit)
	return nil
}
