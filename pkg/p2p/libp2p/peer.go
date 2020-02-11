// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libp2p

import (
	"bytes"
	"sort"
	"sync"

	"github.com/ethersphere/bee/pkg/p2p"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/libp2p/go-libp2p-core/network"
	libp2ppeer "github.com/libp2p/go-libp2p-core/peer"
)

type peerRegistry struct {
	underlays   map[string]libp2ppeer.ID                    // map overlay address to underlay peer id
	overlays    map[libp2ppeer.ID]swarm.Address             // map underlay peer id to overlay address
	connections map[libp2ppeer.ID]map[network.Conn]struct{} // list of connections for safe removal on Disconnect notification
	mu          sync.RWMutex

	network.Notifiee // peerRegistry can be the receiver for network.Notify
}

func newPeerRegistry() *peerRegistry {
	return &peerRegistry{
		underlays:   make(map[string]libp2ppeer.ID),
		overlays:    make(map[libp2ppeer.ID]swarm.Address),
		connections: make(map[libp2ppeer.ID]map[network.Conn]struct{}),
		Notifiee:    new(network.NoopNotifiee),
	}
}

func (r *peerRegistry) Exists(overlay swarm.Address) (found bool) {
	_, found = r.peerID(overlay)
	return found
}

// Disconnect removes the peer from registry in disconnect.
// peerRegistry has to be set by network.Network.Notify().
func (r *peerRegistry) Disconnected(_ network.Network, c network.Conn) {
	peerID := c.RemotePeer()

	r.mu.Lock()
	defer r.mu.Unlock()

	// remove only the related connection,
	// not eventually newly created one for the same peer
	if _, ok := r.connections[peerID][c]; !ok {
		return
	}

	overlay := r.overlays[peerID]
	delete(r.overlays, peerID)
	delete(r.underlays, encodeunderlaysKey(overlay))

	delete(r.connections[peerID], c)
	if len(r.connections[peerID]) == 0 {
		delete(r.connections, peerID)
	}
}

func (r *peerRegistry) peers() []p2p.Peer {
	r.mu.Lock()
	peers := make([]p2p.Peer, 0, len(r.overlays))
	for _, a := range r.overlays {
		peers = append(peers, p2p.Peer{
			Address: a,
		})
	}
	r.mu.Unlock()
	sort.Slice(peers, func(i, j int) bool {
		return bytes.Compare(peers[i].Address.Bytes(), peers[j].Address.Bytes()) == -1
	})
	return peers
}

func (r *peerRegistry) add(c network.Conn, overlay swarm.Address) {
	peerID := c.RemotePeer()

	r.mu.Lock()
	r.underlays[encodeunderlaysKey(overlay)] = peerID
	r.overlays[peerID] = overlay
	if _, ok := r.connections[peerID]; !ok {
		r.connections[peerID] = make(map[network.Conn]struct{})
	}
	r.connections[peerID][c] = struct{}{}
	r.mu.Unlock()
}

func (r *peerRegistry) peerID(overlay swarm.Address) (peerID libp2ppeer.ID, found bool) {
	r.mu.RLock()
	peerID, found = r.underlays[encodeunderlaysKey(overlay)]
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
	delete(r.underlays, encodeunderlaysKey(overlay))
	delete(r.connections, peerID)
	r.mu.Unlock()
}

func encodeunderlaysKey(overlay swarm.Address) string {
	return string(overlay.Bytes())
}
