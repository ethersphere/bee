// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"sync"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// The per-peer stream limiter below is a copy of pkg/pullsync/streamlimit.go
// (rangesync does not import pullsync).

const (
	// maxPeerStatusStreams is the maximum number of concurrent incoming status
	// streams per peer. An honest peer sends one status at a time; this leaves
	// headroom for streams that are closed by the remote but whose handler has
	// not returned yet.
	maxPeerStatusStreams = 4
	// maxPeerReconcileStreams is the maximum number of concurrent incoming
	// reconcile streams per peer. Only MaxConcurrentSessionsPerPeer of them
	// run a session; the others are answered with Busy.
	maxPeerReconcileStreams = 4
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
