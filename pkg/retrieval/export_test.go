// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package retrieval

import (
	"context"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var (
	ProtocolName    = protocolName
	ProtocolVersion = protocolVersion
	StreamName      = streamName
)

func (s *Service) Handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
	return s.handler(ctx, p, stream)
}

func (s *Service) ClosestPeer(addr swarm.Address, skipPeers []swarm.Address, allowUpstream bool) (swarm.Address, error) {
	return s.closestPeer(addr, skipPeers, allowUpstream)
}

// SetHealthFirst makes the origin ask healthy peers at any distance first.
func (s *Service) SetHealthFirst(v bool) { s.healthFirst = v }

// SetForwarderHealthFirst makes a forwarder ask healthy peers at any distance
// first.
func (s *Service) SetForwarderHealthFirst(v bool) { s.forwarderHealthFirst = v }
