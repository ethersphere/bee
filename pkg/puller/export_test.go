// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package puller

import "github.com/ethersphere/bee/v2/pkg/swarm"

type (
	PeerTreeNodeValue = peerTreeNodeValue
)

var (
	PeerIntervalKey = peerIntervalKey
	NewPeerTreeNode = newPeerTreeNode
)

// NewTreeNode is a wrapper for the generic newTreeNode function for testing
func NewTreeNode[T any](key []byte, p *T, level uint8) *TreeNode[T] {
	return newTreeNode(key, p, level)
}

func (p *Puller) IsSyncing(addr swarm.Address) bool {
	p.syncPeersMtx.Lock()
	defer p.syncPeersMtx.Unlock()
	peer, ok := p.syncPeers[addr.ByteString()]
	if !ok {
		return false
	}
	for bin := range p.bins {
		if peer.isBinSyncing(bin) {
			return true
		}
	}
	return false
}

func (p *Puller) IsBinSyncing(addr swarm.Address, bin uint8) bool {
	p.syncPeersMtx.Lock()
	defer p.syncPeersMtx.Unlock()
	if peer, ok := p.syncPeers[addr.ByteString()]; ok {
		return peer.isBinSyncing(bin)
	}
	return false
}
