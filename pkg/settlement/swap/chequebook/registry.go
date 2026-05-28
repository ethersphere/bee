// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chequebook

import (
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/bzz"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// Registry is an in-memory chequebook↔overlay map used by the Verifier
// for the uniqueness check. Entries are removed on peer disconnect so a
// peer can reconnect with a different overlay using the same chequebook.
//
// Implements both AddressbookLookup (for the Verifier) and the hive
// ChequebookStorer (for storing on successful verification).
type Registry struct {
	mu              sync.RWMutex
	byCB            map[common.Address]swarm.Address
	byPeer          map[string]common.Address // key: overlay.ByteString()
	byPeerTimestamp map[string]int64          // key: overlay.ByteString()
}

func NewRegistry() *Registry {
	return &Registry{
		byCB:            make(map[common.Address]swarm.Address),
		byPeer:          make(map[string]common.Address),
		byPeerTimestamp: make(map[string]int64),
	}
}

// Peer returns the overlay currently associated with chequebook.
func (r *Registry) Peer(chequebook common.Address) (swarm.Address, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	overlay, ok := r.byCB[chequebook]
	return overlay, ok
}

// Put atomically updates the in-memory chequebook + last-seen-timestamp
// mapping and runs writeAddressbook (if non-nil) under the same mutex.
// Holding the mutex across the callback prevents an older record from
// overwriting a newer one in the addressbook when two Put calls race for
// the same peer. Zero chequebook skips the registry mapping but still
// runs writeAddressbook.
//
// The callback runs under the registry mutex, so writeAddressbook should
// be a single bounded I/O operation; long-running work belongs outside
// Put.
func (r *Registry) Put(peer swarm.Address, chequebook common.Address, timestamp int64, source bzz.TimestampSource, writeAddressbook func() error) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if (chequebook != common.Address{}) {
		if err := r.putLocked(peer, chequebook, timestamp, source); err != nil {
			return err
		}
	}

	if writeAddressbook == nil {
		return nil
	}

	return writeAddressbook()
}

// putLocked implements Put without taking the mutex; the caller owns r.mu.
func (r *Registry) putLocked(peer swarm.Address, chequebook common.Address, timestamp int64, source bzz.TimestampSource) error {
	key := peer.ByteString()
	var existing *bzz.Address
	if stored, ok := r.byPeerTimestamp[key]; ok {
		existing = &bzz.Address{Timestamp: stored}
	}

	// Re-check timestamp under r.mu against the registry's own last-seen
	// value. The upstream handshake/hive checks compare against the
	// addressbook snapshot taken outside this mutex, so a concurrent Put
	// from the other path may have advanced things since. Without this
	// check an older record could regress a newer one already committed.
	if err := bzz.CheckTimestamp(timestamp, existing, source, time.Now()); err != nil {
		return err
	}

	if prev, ok := r.byPeer[key]; ok && prev != chequebook {
		delete(r.byCB, prev)
	}
	r.byPeer[key] = chequebook
	r.byCB[chequebook] = peer
	r.byPeerTimestamp[key] = timestamp
	return nil
}

// Remove drops any mapping for peer. Safe to call for unknown peers.
func (r *Registry) Remove(peer swarm.Address) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := peer.ByteString()
	if cb, ok := r.byPeer[key]; ok {
		delete(r.byCB, cb)
		delete(r.byPeer, key)
	}
	delete(r.byPeerTimestamp, key)
}
