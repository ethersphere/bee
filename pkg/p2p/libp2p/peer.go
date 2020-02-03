// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"sync"

	"github.com/ethersphere/bee/pkg/swarm"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
)

type peerRegistry struct {
	peers    map[string]libp2ppeer.ID        // map overlay address to underlay peer id
	overlays map[libp2ppeer.ID]swarm.Address // map underlay peer id to overlay address
	mu       sync.RWMutex
}

func newPeerRegistry() *peerRegistry {
	return &peerRegistry{
		peers:    make(map[string]libp2ppeer.ID),
		overlays: make(map[libp2ppeer.ID]swarm.Address),
	}
}

func (r *peerRegistry) Exists(overlay swarm.Address) (found bool) {
	_, found = r.peerID(overlay)
	return found
}

func (r *peerRegistry) add(peerID libp2ppeer.ID, overlay swarm.Address) {
	r.mu.Lock()
	r.peers[encodePeersKey(overlay)] = peerID
	r.overlays[peerID] = overlay
	r.mu.Unlock()
}

func (r *peerRegistry) peerID(overlay swarm.Address) (peerID libp2ppeer.ID, found bool) {
	r.mu.RLock()
	peerID, found = r.peers[encodePeersKey(overlay)]
	r.mu.RUnlock()
	return peerID, found
}

func (r *peerRegistry) overlay(peerID libp2ppeer.ID) (swarm.Address, bool) {
	r.mu.RLock()
	overlay, found := r.overlays[peerID]
	r.mu.RUnlock()
	return overlay, found
}

func (r *peerRegistry) remove(peerID libp2ppeer.ID) {
	r.mu.Lock()
	overlay := r.overlays[peerID]
	delete(r.overlays, peerID)
	delete(r.peers, encodePeersKey(overlay))
	r.mu.Unlock()
}

func encodePeersKey(overlay swarm.Address) string {
	return string(overlay.Bytes())
}
