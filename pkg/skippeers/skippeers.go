// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package skippeers

import (
	"math"
	"sync"
	"time"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const maxDuration time.Duration = math.MaxInt64

type List struct {
	mtx sync.Mutex

	durC chan time.Duration
	quit chan struct{}
	// key is chunk address, value is map of peer address to expiration
	skip map[string]map[string]int64

	wg sync.WaitGroup
}

func NewList() *List {
	l := &List{
		skip: make(map[string]map[string]int64),
		durC: make(chan time.Duration),
		quit: make(chan struct{}),
	}

	l.wg.Add(1)
	go l.worker()

	return l
}

func (l *List) worker() {

	defer l.wg.Done()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.prune()
		case <-l.quit:
			return
		}
	}
}

func (l *List) Forever(chunk, peer swarm.Address) {
	l.Add(chunk, peer, maxDuration)
}

func (l *List) Add(chunk, peer swarm.Address, expire time.Duration) {

	l.mtx.Lock()
	defer l.mtx.Unlock()

	var t int64

	if expire == maxDuration {
		t = maxDuration.Nanoseconds()
	} else {
		t = time.Now().Add(expire).UnixNano()
	}

	if _, ok := l.skip[chunk.ByteString()]; !ok {
		l.skip[chunk.ByteString()] = make(map[string]int64)
	}

	l.skip[chunk.ByteString()][peer.ByteString()] = t
}

func (l *List) ChunkPeers(ch swarm.Address) (peers []swarm.Address) {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	now := time.Now().UnixNano()

	if p, ok := l.skip[ch.ByteString()]; ok {
		peers = make([]swarm.Address, 0, len(p))
		for peer, exp := range p {
			if exp > now {
				peers = append(peers, swarm.NewAddress([]byte(peer)))
			}
		}
	}

	return peers
}

func (l *List) PruneExpiresAfter(ch swarm.Address, d time.Duration) int {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	return l.pruneChunk(ch.ByteString(), time.Now().Add(d).UnixNano())
}

func (l *List) prune() {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	expiresNano := time.Now().UnixNano()
	for k := range l.skip {
		l.pruneChunk(k, expiresNano)
	}
}

// Must be called under lock
func (l *List) pruneChunk(ch string, now int64) int {

	count := 0

	for peer, exp := range l.skip[ch] {
		if exp <= now {
			delete(l.skip[ch], peer)
			count++
		}
	}
	if len(l.skip[ch]) == 0 {
		delete(l.skip, ch)
	}

	return count
}

func (l *List) Close() error {
	close(l.quit)
	l.wg.Wait()

	l.mtx.Lock()
	clear(l.skip)
	l.mtx.Unlock()

	return nil
}
