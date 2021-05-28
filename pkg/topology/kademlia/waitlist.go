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
	dirty              *atomic.Bool // Is marked true when the delete process starts.
	expire             time.Time
	failedConnAttempts int
}

// waitList is a thin wrapper around the sync.Map which main purpose is to
// simplify the compounded operations. It is prohibited to update the waitEntry
// fields separately. The maps' entry must be replaced as a whole.
type waitList struct {
	entries sync.Map
}

// setPeerInfo sets the given peer wait information.
func (wl *waitList) setPeerInfo(addr swarm.Address, expire time.Time, count int) {
	wl.entries.Store(addr.ByteString(), &waitEntry{
		dirty:              atomic.NewBool(false),
		expire:             expire,
		failedConnAttempts: count,
	})
}

// setPeerExpire upserts the expire part of the peer wait information.
func (wl *waitList) setPeerExpire(addr swarm.Address, expire time.Time) {
	val, loaded := wl.entries.LoadOrStore(addr.ByteString(), &waitEntry{
		dirty:  atomic.NewBool(false),
		expire: expire,
	})
	if loaded && !val.(*waitEntry).dirty.Load() {
		wl.entries.Store(addr.ByteString(), &waitEntry{
			dirty:              val.(*waitEntry).dirty,
			expire:             expire,
			failedConnAttempts: val.(*waitEntry).failedConnAttempts,
		})
	}
}

// isPeerBeforeExpire returns ture if the current
// time is before the peers' current expire time.
func (wl *waitList) isPeerBeforeExpire(addr swarm.Address) bool {
	val, ok := wl.entries.Load(addr.ByteString())
	return ok && !val.(*waitEntry).dirty.Load() && time.Now().Before(val.(*waitEntry).expire)
}

// peerFailedConnectionAttempts returns how many failed
// connection attempts were made by the given peer.
func (wl *waitList) peerFailedConnectionAttempts(addr swarm.Address) int {
	val, ok := wl.entries.Load(addr.ByteString())
	if !ok || val.(*waitEntry).dirty.Load() {
		return 0
	}
	return val.(*waitEntry).failedConnAttempts
}

// removePeer removes the given peer from the wait list.
func (wl *waitList) removePeer(addr swarm.Address) {
	val, loaded := wl.entries.LoadAndDelete(addr.ByteString())
	if !loaded {
		return
	}
	val.(*waitEntry).dirty.Store(true)
	wl.entries.Delete(addr.ByteString())
}
