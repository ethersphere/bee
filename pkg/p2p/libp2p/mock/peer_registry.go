// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock

import (
	"sync"

	"github.com/ethersphere/bee/v2/pkg/p2p/libp2p/internal/handshake/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/libp2p/go-libp2p/core/network"
	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"
)

// PeerRegistry is a mock implementation of peerRegistry for testing
type PeerRegistry struct {
	peerMap map[string]mockPeer // overlay address -> mock peer
	mu      sync.RWMutex
}

type mockPeer struct {
	peerID       libp2ppeer.ID
	overlay      swarm.Address
	capabilities *pb.Capabilities
}

// NewPeerRegistry creates a new mock peer registry
func NewPeerRegistry() *PeerRegistry {
	return &PeerRegistry{
		peerMap: make(map[string]mockPeer),
	}
}

// AddPeer adds a peer with given capabilities to the mock registry
func (m *PeerRegistry) AddPeer(overlay swarm.Address, peerID libp2ppeer.ID, capabilities *pb.Capabilities) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.peerMap[overlay.ByteString()] = mockPeer{
		peerID:       peerID,
		overlay:      overlay,
		capabilities: capabilities,
	}
}

// PeerID returns the peer ID for a given overlay address
func (m *PeerRegistry) PeerID(overlay swarm.Address) (libp2ppeer.ID, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	peer, found := m.peerMap[overlay.ByteString()]
	return peer.peerID, found
}

// Capabilities returns the capabilities for a given overlay address
func (m *PeerRegistry) Capabilities(overlay swarm.Address) *pb.Capabilities {
	m.mu.RLock()
	defer m.mu.RUnlock()
	peer, found := m.peerMap[overlay.ByteString()]
	if !found {
		return nil
	}
	return peer.capabilities
}

// Mock interface methods required by peerRegistry
func (m *PeerRegistry) Exists(overlay swarm.Address) bool              { return true }
func (m *PeerRegistry) Disconnected(_ network.Network, c network.Conn) {}
