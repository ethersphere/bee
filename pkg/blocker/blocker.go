// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package blocker

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
)

// maxTimerDelay represents the maximum acceptable delay caused by the
// timer inaccuracy, see: https://github.com/golang/go/issues/14410.
var maxTimerDelay = time.Second

type peer struct {
	blockAfter time.Time // timestamp of the point we've timed-out or got an error from a peer
	addr       swarm.Address
}

type Blocker struct {
	mu                sync.Mutex
	lastRun           time.Time
	contChan          chan os.Signal
	disconnector      p2p.Blocklister
	flagTimeout       time.Duration // how long before blocking a flagged peer
	blockDuration     time.Duration // how long to blocklist a bad peer
	workerWakeup      time.Duration // how often to wake up and block/noop/unblock
	peers             map[string]*peer
	logger            logging.Logger
	wakeupCh          chan struct{}
	quit              chan struct{}
	closeWg           sync.WaitGroup
	blocklistCallback func(swarm.Address)
}

func New(dis p2p.Blocklister, flagTimeout, blockDuration, wakeUpTime time.Duration, callback func(swarm.Address), logger logging.Logger) *Blocker {
	if wakeUpTime <= maxTimerDelay {
		panic(errors.New("worker wake-up time must be bigger then the acceptable timer delay"))
	}
	if blockDuration <= wakeUpTime {
		panic(errors.New("block duration must be bigger then the worker wake-up time"))
	}

	b := &Blocker{
		contChan:          make(chan os.Signal, 1),
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

	signal.Notify(b.contChan, syscall.SIGCONT)
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
			fmt.Printf("block now %v\n", time.Now())
			fmt.Printf("block last %v\n", b.lastRun)
			b.block()
			b.lastRun = time.Now()
		case <-b.contChan:
			fmt.Printf("signal now %v\n", time.Now())
			fmt.Printf("signal last %v\n", b.lastRun)
			// We detect the hibernation/sleep indirectly
			// by checking if the last run is greater than
			// the periodic execution of the worker. This
			// way we ignore the false SIGCONT signals.
			diff := time.Since(b.lastRun)
			fmt.Printf("diff %v\n", b.lastRun)
			if diff > b.workerWakeup+maxTimerDelay {
				b.rewriteFlagTime(diff)
			}
		}
	}
}

func (b *Blocker) rewriteFlagTime(diff time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, peer := range b.peers {
		fmt.Printf("current %v\n", peer.blockAfter)
		peer.blockAfter.Add(diff)
		fmt.Printf("new %v\n", peer.blockAfter)
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

func (b *Blocker) PruneUnseen(seen []swarm.Address) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for a := range b.peers {
		if !isIn(a, seen) {
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

func isIn(addr string, addrs []swarm.Address) bool {
	for _, a := range addrs {
		if a.ByteString() == addr {
			return true
		}
	}
	return false
}
