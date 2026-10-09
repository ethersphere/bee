// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullsync

import (
	"sync"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	// maxPeerSyncStreams is the maximum number of concurrent incoming pullsync
	// streams per peer. An honest puller opens at most one historical and one
	// live stream per bin (2 * swarm.MaxBins), this leaves headroom for streams
	// that are closed by the remote but whose handler has not returned yet.
	maxPeerSyncStreams = 128
	// maxPeerCursorStreams is the maximum number of concurrent incoming cursor
	// streams per peer.
	maxPeerCursorStreams = 4
)

// peerStreamLimiter limits the number of concurrent streams per peer using a
// token pool (buffered channel) per peer.
type peerStreamLimiter struct {
	mu    sync.Mutex
	limit int
	pools map[string]chan struct{}
}

func newPeerStreamLimiter(limit int) *peerStreamLimiter {
	return &peerStreamLimiter{
		limit: limit,
		pools: make(map[string]chan struct{}),
	}
}

// acquire tries to take a slot for the peer without blocking. If successful,
// the returned release function must be called to free the slot.
func (l *peerStreamLimiter) acquire(peer swarm.Address) (release func(), ok bool) {
	key := peer.ByteString()

	l.mu.Lock()
	pool, exists := l.pools[key]
	if !exists {
		pool = make(chan struct{}, l.limit)
		l.pools[key] = pool
	}
	l.mu.Unlock()

	select {
	case pool <- struct{}{}:
		return func() { <-pool }, true
	default:
		return nil, false
	}
}

// clear removes the peer's pool. Slots held at that time are released to the
// removed pool.
func (l *peerStreamLimiter) clear(peer swarm.Address) {
	l.mu.Lock()
	delete(l.pools, peer.ByteString())
	l.mu.Unlock()
}
