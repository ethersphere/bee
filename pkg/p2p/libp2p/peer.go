// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"sync"

	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
)

type peerRegistry struct {
	peers    map[string]libp2ppeer.ID
	overlays map[libp2ppeer.ID]string
	mu       sync.RWMutex
}

func newPeerRegistry() *peerRegistry {
	return &peerRegistry{
		peers:    make(map[string]libp2ppeer.ID),
		overlays: make(map[libp2ppeer.ID]string),
	}
}

func (r *peerRegistry) add(peerID libp2ppeer.ID, overlay string) {
	r.mu.Lock()
	r.peers[overlay] = peerID
	r.overlays[peerID] = overlay
	r.mu.Unlock()
}

func (r *peerRegistry) peerID(overlay string) (peerID libp2ppeer.ID, found bool) {
	r.mu.RLock()
	peerID, found = r.peers[overlay]
	r.mu.RUnlock()
	return peerID, found
}

func (r *peerRegistry) overlay(peerID libp2ppeer.ID) (overlay string, found bool) {
	r.mu.RLock()
	overlay, found = r.overlays[peerID]
	r.mu.RUnlock()
	return overlay, found
}
