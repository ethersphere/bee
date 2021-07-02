// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pushsync

import "github.com/ethersphere/bee/pkg/swarm"

var (
	ProtocolName     = protocolName
	ProtocolVersion  = protocolVersion
	StreamName       = streamName
	NewPeerSkipList  = newPeerSkipList
	DefaultTtl       = &defaultTTL
	SendReceiptDelay = &sendReceiptDelay
)

func (ps *PushSync) ShouldSkip(peer swarm.Address) bool {
	return ps.skipList.ShouldSkip(peer)
}
