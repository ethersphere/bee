// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package blocker

import (
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
)

type peer struct {
	blockAfter time.Time // timestamp of the point we've timed-out or got an error from a peer
	addr       swarm.Address
}

type Blocker struct {
	mux               sync.Mutex
	disconnector      p2p.Blocklister
	flagTimeout       time.Duration // how long before blocking a flagged peer
	blockDuration     time.Duration // how long to blocklist a bad peer
	workerWakeup      time.Duration // how long to blocklist a bad peer
	peers             map[string]*peer
	logger            logging.Logger
	wakeupCh          chan struct{}
	quit              chan struct{}
	closeWg           sync.WaitGroup
	blocklistCallback func(swarm.Address)
}

func New(dis p2p.Blocklister, flagTimeout, blockDuration, wakeUpTime time.Duration, callback func(swarm.Address), logger logging.Logger) *Blocker {
	b := &Blocker{
		disconnector:      dis,
		flagTimeout:       flagTimeout,
		blockDuration:     blockDuration,
		workerWakeup:      wakeUpTime,
		peers:             map[string]*peer{},
		wakeupCh:          make(chan struct{}),
		quit:              make(chan struct{}),
		logger:            logger,
		closeWg:           sync.WaitGroup{},
		blocklistCallback: callback,
	}

	b.closeWg.Add(1)
	go b.run()

	return b
}

func (b *Blocker) run() {
	defer b.closeWg.Done()
	for {
		select {
		case <-b.quit:
			return
		case <-time.After(b.workerWakeup):
			b.block()
		}
	}
}

func (b *Blocker) block() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for key, peer := range b.peers {

		select {
		case <-b.quit:
			return
		default:
		}

		if !peer.blockAfter.IsZero() && time.Now().After(peer.blockAfter) {
			if err := b.disconnector.Blocklist(peer.addr, b.blockDuration, "blocker: flag timeout"); err != nil {
				b.logger.Warningf("blocker: blocking peer %s failed: %v", peer.addr, err)
			}
			if b.blocklistCallback != nil {
				b.blocklistCallback(peer.addr)
			}
			delete(b.peers, key)
		}
	}
}

func (b *Blocker) Flag(addr swarm.Address) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.peers[addr.ByteString()]; !ok {
		b.peers[addr.ByteString()] = &peer{
			blockAfter: time.Now().Add(b.flagTimeout),
			addr:       addr,
		}
	}
}

func (b *Blocker) Unflag(addr swarm.Address) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.peers, addr.ByteString())
}

// Close will exit the worker loop.
// must be called only once.
func (b *Blocker) Close() error {
	close(b.quit)
	b.closeWg.Wait()
	return nil
}
