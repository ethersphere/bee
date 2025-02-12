// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package blocker

import (
	"fmt"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"go.uber.org/atomic"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "blocker"

// sequencerResolution represents monotonic sequencer resolution.
// It must be in the time.Duration base form without a multiplier.
var sequencerResolution = time.Second

type peer struct {
	blockAfter uint64
	address    swarm.Address
}

type Blocker struct {
	sequence          atomic.Uint64 // Monotonic clock.
	mu                sync.Mutex
	blocklister       p2p.Blocklister
	flagTimeout       time.Duration // how long before blocking a flagged peer
	blockDuration     time.Duration // how long to blocklist a bad peer
	peers             map[string]*peer
	logger            log.Logger
	wakeupCh          chan struct{}
	quit              chan struct{}
	closeWg           sync.WaitGroup
	blocklistCallback func(swarm.Address)
}

func New(blocklister p2p.Blocklister, flagTimeout, blockDuration, wakeUpTime time.Duration, callback func(swarm.Address), logger log.Logger) *Blocker {
	if flagTimeout <= sequencerResolution {
		panic(fmt.Errorf("flag timeout %v cannot be equal to or lower then the sequencer resolution %v", flagTimeout, sequencerResolution))
	}
	if wakeUpTime < sequencerResolution {
		panic(fmt.Errorf("wakeup time %v cannot be lower then the clock sequencer resolution %v", wakeUpTime, sequencerResolution))
	}

	b := &Blocker{
		blocklister:       blocklister,
		flagTimeout:       flagTimeout,
		blockDuration:     blockDuration,
		peers:             map[string]*peer{},
		wakeupCh:          make(chan struct{}),
		quit:              make(chan struct{}),
		logger:            logger.WithName(loggerName).Register(),
		closeWg:           sync.WaitGroup{},
		blocklistCallback: callback,
	}

	b.closeWg.Add(1)
	go func() {
		defer b.closeWg.Done()
		for {
			select {
			case <-b.quit:
				return
			case <-time.After(sequencerResolution):
				if b.blocklister.NetworkStatus() == p2p.NetworkStatusAvailable {
					b.sequence.Inc()
				}
			}
		}
	}()

	b.closeWg.Add(1)
	go func() {
		defer b.closeWg.Done()
		for {
			select {
			case <-time.After(wakeUpTime):
				b.block()
			case <-b.quit:
				return
			}
		}
	}()

	return b
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

		if 0 < peer.blockAfter && peer.blockAfter < b.sequence.Load() {
			if err := b.blocklister.Blocklist(peer.address, b.blockDuration, "blocker: flag timeout"); err != nil {
				b.logger.Warning("blocking peer failed", "peer_address", peer.address, "error", err)
			}
			if b.blocklistCallback != nil {
				b.blocklistCallback(peer.address)
			}
			delete(b.peers, key)
		}
	}
}

func (b *Blocker) Flag(addr swarm.Address) {
	if b.blocklister.NetworkStatus() != p2p.NetworkStatusAvailable {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.peers[addr.ByteString()]; !ok {
		b.peers[addr.ByteString()] = &peer{
			blockAfter: b.sequence.Load() + uint64(b.flagTimeout/sequencerResolution),
			address:    addr,
		}
	}
}

func (b *Blocker) Unflag(addr swarm.Address) {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.peers, addr.ByteString())
}

func (b *Blocker) PruneUnseen(seen []swarm.Address) {
	isSeen := func(addr string) bool {
		for _, a := range seen {
			if a.ByteString() == addr {
				return true
			}
		}
		return false
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	for a := range b.peers {
		if !isSeen(a) {
			delete(b.peers, a)
		}
	}
}

// Close will exit the worker loop.
// must be called only once.
func (b *Blocker) Close() error {
	close(b.quit)
	b.closeWg.Wait()
	return nil
}
