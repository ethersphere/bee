// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package puller

import "github.com/ethersphere/bee/v2/pkg/swarm"

var PeerIntervalKey = peerIntervalKey

func (p *Puller) IsSyncing(addr swarm.Address) bool {
	p.syncPeersMtx.Lock()
	defer p.syncPeersMtx.Unlock()
	_, ok := p.syncPeers[addr.ByteString()]
	return ok
}

func (p *Puller) IsBinSyncing(addr swarm.Address, bin uint8) bool {
	p.syncPeersMtx.Lock()
	defer p.syncPeersMtx.Unlock()
	if peer, ok := p.syncPeers[addr.ByteString()]; ok {
		return peer.isBinSyncing(bin)
	}
	return false
}
