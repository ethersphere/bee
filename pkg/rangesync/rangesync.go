// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"context"
	"errors"
	"io"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/p2p"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "rangesync"

// Protocol and stream names (proposal §6.1).
const (
	ProtocolName    = "rangesync"
	ProtocolVersion = "1.0.0"

	// StatusStreamName is the stream on which peers gossip their root summary.
	StatusStreamName = "status"
	// ReconcileStreamName is the stream of one reconciliation session, opened
	// by the initiator.
	ReconcileStreamName = "reconcile"
	// FetchStreamName is the stream on which a requester asks for chunks by ID
	// and the holder delivers them.
	FetchStreamName = "fetch"
)

var errNotImplemented = errors.New("rangesync: not implemented")

var _ io.Closer = (*Service)(nil)

// Service is the rangesync protocol service. It serves the status, reconcile
// and fetch streams and opens them towards peers.
type Service struct {
	streamer p2p.Streamer
	logger   log.Logger
}

// New returns a new rangesync Service that opens streams through streamer.
func New(streamer p2p.Streamer, logger log.Logger) *Service {
	return &Service{
		streamer: streamer,
		logger:   logger.WithName(loggerName).Register(),
	}
}

// Protocol returns the p2p protocol specification of rangesync: protocol
// ProtocolName, version ProtocolVersion, with the status, reconcile and fetch
// streams.
func (s *Service) Protocol() p2p.ProtocolSpec {
	return p2p.ProtocolSpec{
		Name:    ProtocolName,
		Version: ProtocolVersion,
		StreamSpecs: []p2p.StreamSpec{
			{
				Name:    StatusStreamName,
				Handler: s.handleStatus,
			},
			{
				Name:    ReconcileStreamName,
				Handler: s.handleReconcile,
			},
			{
				Name:    FetchStreamName,
				Handler: s.handleFetch,
			},
		},
	}
}

// Close stops the service.
func (s *Service) Close() error {
	return nil
}

// handleStatus serves the status stream. Until it is implemented, every
// handler resets the stream and returns errNotImplemented.
func (s *Service) handleStatus(_ context.Context, _ p2p.Peer, stream p2p.Stream) error {
	_ = stream.Reset()
	return errNotImplemented
}

// handleReconcile serves the reconcile stream. A non-zero Message.AuditRound
// (salted session) is ignored until salted sessions are implemented.
func (s *Service) handleReconcile(_ context.Context, _ p2p.Peer, stream p2p.Stream) error {
	_ = stream.Reset()
	return errNotImplemented
}

// handleFetch serves the fetch stream.
func (s *Service) handleFetch(_ context.Context, _ p2p.Peer, stream p2p.Stream) error {
	_ = stream.Reset()
	return errNotImplemented
}
