// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullsync

import (
	"context"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	dto "github.com/prometheus/client_model/go"
)

// Exported protocol identifiers for tests and fuzzers that need to open or
// register the pullsync stream through the p2p streamtest recorder.
const (
	ProtocolName    = protocolName
	ProtocolVersion = protocolVersion
	StreamName      = streamName
)

const (
	MaxPeerSyncStreams   = maxPeerSyncStreams
	MaxPeerCursorStreams = maxPeerCursorStreams
)

var (
	ErrStreamLimitExceeded = errStreamLimitExceeded
	NewPeerStreamLimiter   = newPeerStreamLimiter
)

func (l *peerStreamLimiter) Acquire(peer swarm.Address) (func(), bool) { return l.acquire(peer) }
func (l *peerStreamLimiter) Clear(peer swarm.Address)                  { l.clear(peer) }

// Handler exposes the pullsync stream handler.
func (s *Syncer) Handler(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
	return s.handler(ctx, p, stream)
}

// CursorHandler exposes the cursor stream handler.
func (s *Syncer) CursorHandler(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
	return s.cursorHandler(ctx, p, stream)
}

// FillSyncStreams takes all sync stream slots of the peer.
func (s *Syncer) FillSyncStreams(peer swarm.Address) {
	for range maxPeerSyncStreams {
		s.syncStreams.acquire(peer)
	}
}

// FillCursorStreams takes all cursor stream slots of the peer.
func (s *Syncer) FillCursorStreams(peer swarm.Address) {
	for range maxPeerCursorStreams {
		s.cursorStreams.acquire(peer)
	}
}

// StreamLimitExceededCount returns the stream limit exceeded metric value.
func (s *Syncer) StreamLimitExceededCount() float64 {
	var m dto.Metric
	if err := s.metrics.StreamLimitExceeded.Write(&m); err != nil {
		return -1
	}
	return m.GetCounter().GetValue()
}
