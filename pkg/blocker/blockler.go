// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package blocker

import (
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
)

type peer struct {
	flagged    bool      // indicates whether the peer is actively flagged
	blockAfter time.Time // timestamp of the point we've timed-out or got an error from a peer
	addr       swarm.Address
}

type Blocker struct {
	disconnector  p2p.Blocklister
	flagTimeout   time.Duration // how long before blocking a flagged peer
	blockDuration time.Duration // how long to blocklist a bad peer
	peers         map[string]*peer
	logger        logging.Logger
	add           chan swarm.Address
	remove        chan swarm.Address
	quit          chan struct{}
}

func New(dis p2p.Blocklister, flagTimeout, blockDuration time.Duration, logger logging.Logger) *Blocker {

	b := &Blocker{
		disconnector:  dis,
		flagTimeout:   flagTimeout,
		blockDuration: blockDuration,
		peers:         map[string]*peer{},
		add:           make(chan swarm.Address),
		remove:        make(chan swarm.Address),
		quit:          make(chan struct{}),
		logger:        logger,
	}

	go b.run()

	return b
}

func (b *Blocker) run() {

	for {
		select {
		case <-b.quit:
			return
		case addr := <-b.add:
			b.addToPending(addr)
		case addr := <-b.remove:
			delete(b.peers, addr.ByteString())
		case <-time.After(b.flagTimeout):
			b.blockPending()
		}
	}
}

func (b *Blocker) addToPending(addr swarm.Address) {
	p, ok := b.peers[addr.ByteString()]
	if ok {
		if !p.flagged {
			p.blockAfter = time.Now().Add(b.flagTimeout)
			p.flagged = true
		}
	} else {
		b.peers[addr.ByteString()] = &peer{
			blockAfter: time.Now().Add(b.flagTimeout),
			flagged:    true,
			addr:       addr,
		}
	}
}
func (b *Blocker) blockPending() {
	for key, peer := range b.peers {
		if peer.flagged && time.Now().After(peer.blockAfter) {
			if err := b.disconnector.Blocklist(peer.addr, b.blockDuration, "blocker: flag timeout"); err != nil {
				b.logger.Warningf("blocker: blocking peer %s failed: %v", peer.addr, err)
			}

			delete(b.peers, key)
		}
	}
}

func (b *Blocker) Flag(addr swarm.Address) {
	b.add <- addr
}

func (b *Blocker) Unflag(addr swarm.Address) {
	b.remove <- addr
}

func (b *Blocker) Close() error {
	close(b.quit)
	close(b.add)
	close(b.remove)
	return nil
}
