// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kademlia

import (
	"sync"
	"time"

	"github.com/ethersphere/bee/pkg/swarm"
	"go.uber.org/atomic"
)

// waitEntry holds information related to the current peer wait status.
type waitEntry struct {
	expire             *atomic.Int64
	failedConnAttempts int
}

// waitList is a thin wrapper around the sync.Map which main
// purpose is to simplify the compounded operations.
type waitList struct {
	entries sync.Map
}

// setPeerInfo upserts the given peer wait information.
func (wl *waitList) setPeerInfo(addr swarm.Address, expire time.Time, count int) {
	wl.entries.Store(addr.ByteString(), &waitEntry{
		expire:             atomic.NewInt64(expire.UnixNano()),
		failedConnAttempts: count,
	})
}

// setPeerExpire upserts the expire part of the peer wait information.
func (wl *waitList) setPeerExpire(addr swarm.Address, expire time.Time) {
	val, loaded := wl.entries.LoadOrStore(addr.ByteString(), &waitEntry{
		expire: atomic.NewInt64(expire.UnixNano()),
	})
	if loaded {
		val.(*waitEntry).expire.Store(expire.UnixNano())
	}
}

// isPeerBeforeExpire returns ture if the current
// time is before the peers' current expire time.
func (wl *waitList) isPeerBeforeExpire(addr swarm.Address) bool {
	val, ok := wl.entries.Load(addr.ByteString())
	return ok && time.Now().UnixNano() < val.(*waitEntry).expire.Load()
}

// peerFailedConnectionAttempts returns how many failed
// connection attempts were made by the given peer.
func (wl *waitList) peerFailedConnectionAttempts(addr swarm.Address) int {
	val, ok := wl.entries.Load(addr.ByteString())
	if !ok {
		return 0
	}
	return val.(*waitEntry).failedConnAttempts
}

// removePeer removes the given peer from the wait list.
func (wl *waitList) removePeer(addr swarm.Address) {
	wl.entries.Delete(addr.ByteString())
}
