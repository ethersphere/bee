// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// statusTimeout bounds the exchange of one Status.
const statusTimeout = 10 * time.Second

// localStatus returns this node's Status: the root and summary of the index
// and its leaf bits, with fingerprint.DefaultMapperID. It returns an error
// wrapping fingerprint.ErrBusy until the index is built.
func (s *Service) localStatus() (Status, error) {
	root, sum, err := s.index.RootSummary()
	if err != nil {
		return Status{}, fmt.Errorf("root summary: %w", err)
	}
	return Status{
		Root:     root,
		Summary:  sum,
		LeafBits: s.index.LeafBits(),
		Mapping:  fingerprint.DefaultMapperID,
	}, nil
}

// SendStatus sends this node's Status to peer on a status stream. It fails
// with an error wrapping fingerprint.ErrBusy until the index is built. When
// to send is the caller's decision (Phase 4 gossips on root changes and as a
// heartbeat).
func (s *Service) SendStatus(ctx context.Context, peer swarm.Address) (err error) {
	st, err := s.localStatus()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()
	stream, err := s.streamer.NewStream(ctx, peer, nil, ProtocolName, ProtocolVersion, StatusStreamName)
	if err != nil {
		return fmt.Errorf("new stream: %w", err)
	}
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			_ = stream.FullClose()
		}
	}()
	if err := protobuf.NewWriter(stream).WriteMsgWithContext(ctx, EncodeStatus(st)); err != nil {
		return fmt.Errorf("write status: %w", err)
	}
	return nil
}

// LastStatus returns the last Status received from peer, if any. It is
// forgotten when the peer disconnects.
func (s *Service) LastStatus(peer swarm.Address) (Status, bool) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	st, ok := s.statuses[peer.ByteString()]
	return st, ok
}

// handleStatus serves the status stream: it reads one Status and records it
// as the peer's last status. An invalid Status disconnects the peer. A Status
// whose Mapping differs from this node's is recorded and counted
// (mapping_mismatch_total); sessions with that peer are refused.
func (s *Service) handleStatus(ctx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	release, ok := s.statusStreams.acquire(p.Address)
	if !ok {
		return s.streamLimitExceeded(p.Address, stream, StatusStreamName, maxPeerStatusStreams)
	}
	defer release()

	ctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()

	var m pb.Status
	if err := readMessage(ctx, newReader(stream, maxStatusBytes), &m); err != nil {
		if errors.Is(err, ErrInvalidMessage) {
			return s.violation(p.Address, stream, fmt.Errorf("status: %w", err))
		}
		_ = stream.Reset()
		return fmt.Errorf("read status: %w", err)
	}
	st, err := ParseStatus(&m)
	if err != nil {
		return s.violation(p.Address, stream, err)
	}
	if st.Mapping != fingerprint.DefaultMapperID {
		s.metrics.MappingMismatch.Inc()
		s.logger.Debug("peer status mapping differs", "peer_address", p.Address, "mapping", st.Mapping)
	}
	// This can race with the disconnected hook and leave the Status of a
	// peer that just disconnected. That is accepted: the entry is one small
	// value per peer address, it is replaced by the peer's next Status or
	// deleted on its next disconnect, and a Reconcile using it fails at
	// NewStream because the peer is not connected.
	s.statusMu.Lock()
	s.statuses[p.Address.ByteString()] = st
	s.statusMu.Unlock()
	return stream.FullClose()
}
