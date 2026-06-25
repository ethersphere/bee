// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hive

import (
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	defaultGossipDedupTTL           = time.Minute
	defaultGossipDedupPruneInterval = time.Minute
)

// gossipDedupCache suppresses repeated outbound gossip of the same peer to the same
// addressee within a TTL window.
type gossipDedupCache struct {
	mu   sync.Mutex
	seen map[string]map[string]int64 // addressee -> gossiped peer -> expiry unix nano
	ttl  time.Duration
	quit chan struct{}
	wg   sync.WaitGroup
}

func newGossipDedupCache(ttl, pruneInterval time.Duration) *gossipDedupCache {
	if ttl == 0 {
		ttl = defaultGossipDedupTTL
	}

	if pruneInterval == 0 {
		pruneInterval = defaultGossipDedupPruneInterval
	}

	d := &gossipDedupCache{
		seen: make(map[string]map[string]int64),
		ttl:  ttl,
		quit: make(chan struct{}),
	}

	d.wg.Go(func() {
		ticker := time.NewTicker(pruneInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				d.prune()
			case <-d.quit:
				return
			}
		}
	})

	return d
}

func (d *gossipDedupCache) contains(addressee, peer swarm.Address) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	peers, ok := d.seen[addressee.ByteString()]
	if !ok {
		return false
	}

	exp, ok := peers[peer.ByteString()]
	if !ok {
		return false
	}

	return exp > time.Now().UnixNano()
}

func (d *gossipDedupCache) add(addressee swarm.Address, peers ...swarm.Address) {
	if len(peers) == 0 {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	exp := time.Now().Add(d.ttl).UnixNano()
	key := addressee.ByteString()

	m, ok := d.seen[key]
	if !ok {
		m = make(map[string]int64)
		d.seen[key] = m
	}

	for _, p := range peers {
		m[p.ByteString()] = exp
	}
}

func (d *gossipDedupCache) clearAddressee(addressee swarm.Address) {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.seen, addressee.ByteString())
}

func (d *gossipDedupCache) prune() {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now().UnixNano()
	for addressee, peers := range d.seen {
		for peer, exp := range peers {
			if exp <= now {
				delete(peers, peer)
			}
		}
		if len(peers) == 0 {
			delete(d.seen, addressee)
		}
	}
}

func (d *gossipDedupCache) close() {
	close(d.quit)
	d.wg.Wait()

	d.mu.Lock()
	clear(d.seen)
	d.mu.Unlock()
}
