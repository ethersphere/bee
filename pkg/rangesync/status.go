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

// errStatusRateLimit is returned by the status handler for a Status the peer
// sent over its rate (PeerStatusBurst, PeerStatusInterval).
var errStatusRateLimit = errors.New("rangesync: status rate limit exceeded")

// LocalStatus returns this node's Status: the root and summary of the index
// and its leaf bits, with fingerprint.DefaultMapperID. It returns an error
// wrapping fingerprint.ErrBusy until the index is built.
func (s *Service) LocalStatus() (Status, error) {
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

// SendStatus sends this node's current Status (LocalStatus) to peer, as
// SendStatusWith does. It fails with an error wrapping fingerprint.ErrBusy
// until the index is built.
func (s *Service) SendStatus(ctx context.Context, peer swarm.Address) error {
	st, err := s.LocalStatus()
	if err != nil {
		return err
	}
	return s.SendStatusWith(ctx, peer, st)
}

// SendStatusWith sends st, a Status this node got from LocalStatus, to peer
// on a status stream. When to send is the caller's decision (the reconciler
// gossips on root changes and as a heartbeat). It reports a send as done only
// once the peer closed the stream: a peer that dropped the Status (over its
// rate, or invalid) resets it, and the error says so. When the peer does not
// support the protocol the error wraps ErrProtocolUnsupported.
func (s *Service) SendStatusWith(ctx context.Context, peer swarm.Address, st Status) (err error) {
	ctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()
	stream, err := s.streamer.NewStream(ctx, peer, nil, ProtocolName, ProtocolVersion, StatusStreamName)
	if err != nil {
		if _, ok := errors.AsType[*p2p.IncompatibleStreamError](err); ok {
			return fmt.Errorf("%w: %w", ErrProtocolUnsupported, err)
		}
		return fmt.Errorf("new stream: %w", err)
	}
	defer func() {
		if err != nil {
			_ = stream.Reset()
			return
		}
		// FullClose waits for the peer to close its side: EOF if it read the
		// Status, an error if it reset the stream.
		if cerr := stream.FullClose(); cerr != nil {
			err = fmt.Errorf("close status stream: %w", cerr)
		}
	}()
	m := EncodeStatus(st)
	if err := protobuf.NewWriter(stream).WriteMsgWithContext(ctx, m); err != nil {
		return fmt.Errorf("write status: %w", err)
	}
	s.metrics.BytesMetadata.WithLabelValues(dirOut).Add(float64(wireSize(m)))
	return nil
}

// SubscribeRootChange returns a channel that receives a value after this
// node's root summary may have changed, debounced to StatusMinInterval (see
// fingerprint.Index.SubscribeRootChange). The reconciler pushes its Status
// on it.
func (s *Service) SubscribeRootChange() (c <-chan struct{}, unsubscribe func()) {
	return s.index.SubscribeRootChange()
}

// LastStatus returns the last Status received from peer, if any. It is kept
// in memory only and forgotten when the peer disconnects.
func (s *Service) LastStatus(peer swarm.Address) (Status, bool) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	st, ok := s.statuses[peer.ByteString()]
	return st, ok
}

// handleStatus serves the status stream: it reads one Status and records it
// as the peer's last status. An invalid Status disconnects the peer. A Status
// whose Mapping differs from this node's is recorded and counted
// (mapping_mismatch_total); sessions with that peer are refused. A Status
// over the peer's rate (PeerStatusBurst, then one per PeerStatusInterval) is
// dropped unread and counted (status_rate_limited_total): the stream is
// reset, so the sender sees a failed send and retries later, and the peer is
// neither disconnected nor scored.
func (s *Service) handleStatus(ctx context.Context, p p2p.Peer, stream p2p.Stream) (err error) {
	release, ok := s.statusStreams.acquire(p.Address)
	if !ok {
		return s.streamLimitExceeded(p.Address, stream, StatusStreamName, maxPeerStatusStreams)
	}
	defer release()
	// As with the session limiter, this can recreate the entry of a peer
	// that just disconnected; it is cleared on its next disconnect.
	if !s.statusLimiter.Allow(p.Address.ByteString(), 1) {
		_ = stream.Reset()
		s.metrics.StatusRateLimited.Inc()
		s.logger.Debug("peer status over the rate, dropped", "peer_address", p.Address)
		return errStatusRateLimit
	}

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
	s.metrics.BytesMetadata.WithLabelValues(dirIn).Add(float64(wireSize(&m)))
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
