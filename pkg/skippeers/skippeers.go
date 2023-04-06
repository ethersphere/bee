// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package skippeers

import (
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/swarm"
)

type List struct {
	mtx sync.Mutex

	// key is chunk address, value is map of peer address to expiration
	skip map[string]map[string]int64
}

func NewList() *List {
	return &List{
		skip: make(map[string]map[string]int64),
	}
}

func (l *List) Add(chunk, peer swarm.Address, expire time.Duration) {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	if _, ok := l.skip[chunk.ByteString()]; !ok {
		l.skip[chunk.ByteString()] = make(map[string]int64)
	}

	l.skip[chunk.ByteString()][peer.ByteString()] = time.Now().Add(expire).UnixMilli()
}

func (l *List) ChunkPeers(ch swarm.Address) (peers []swarm.Address) {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	now := time.Now().UnixMilli()

	if p, ok := l.skip[ch.ByteString()]; ok {
		for peer, exp := range p {
			if exp > now {
				peers = append(peers, swarm.NewAddress([]byte(peer)))
			}
		}
	}
	return peers
}

func (l *List) PruneExpiresAfter(d time.Duration) int {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	now := time.Now().Add(d).UnixMilli()
	count := 0

	for k, chunkPeers := range l.skip {
		chunkPeersLen := len(chunkPeers)
		for peer, exp := range chunkPeers {
			if exp <= now {
				delete(chunkPeers, peer)
				count++
				chunkPeersLen--
			}
		}
		// prune the chunk too
		if chunkPeersLen == 0 {
			delete(l.skip, k)
		}
	}

	return count
}

func (l *List) Reset() {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	for k := range l.skip {
		delete(l.skip, k)
	}
}
