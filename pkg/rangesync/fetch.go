// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/p2p"
	"github.com/ethersphere/bee/v2/pkg/p2p/protobuf"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/pb"
	"github.com/ethersphere/bee/v2/pkg/rangesync/wantset"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

const (
	// maxPeerFetchStreams is the maximum number of concurrent incoming fetch
	// streams per peer. A requester with MaxClaimsPerPeer claims has at most
	// four Wants of MaxWantIDs outstanding.
	maxPeerFetchStreams = 4

	// handleMaxChunksPerSecond is the number of chunks per second the fetch
	// handler delivers to one peer, the same budget as pullsync's handler.
	handleMaxChunksPerSecond = 250
	// handleRateLimit is the interval of one delivered chunk per peer.
	handleRateLimit = time.Second / handleMaxChunksPerSecond

	// maxWantBytes bounds the encoded size of one Want: MaxWantIDs IDs of
	// about 70 bytes each fit with room to spare.
	maxWantBytes = 32 * 1024
	// maxDeliveryBytes bounds the encoded size of one Delivery: an address,
	// the largest chunk (a SOC) and a stamp with their field overhead.
	maxDeliveryBytes = swarm.HashSize + swarm.SocMaxChunkSize + postage.StampSize + 64

	// fetchKey is the key of the node-wide fetch limiter.
	fetchKey = ""
)

// errNoStampValidator rejects every delivery of a Service created without a
// stamp validator.
var errNoStampValidator = errors.New("rangesync: no stamp validator")

// Fetch requests the chunks with the given IDs (batch IDs are ignored) from
// peer on fetch streams, MaxWantIDs per Want, and stores the valid
// deliveries in the reserve. It returns the number of chunks stored.
//
// Before each Want it drops the IDs outside the storage radius, the IDs the
// reserve holds already and the IDs the want set does not grant; when none
// remains no stream is opened. The claims of an ID end with Done when it was
// stored (or a newer stamp is held) and with Release otherwise, on every
// return. The node-wide fetch rate (Options.FetchRate) is waited for before
// the IDs of each Want are claimed. A holder that has not finished within
// Options.FetchTimeout of the Want is a violation. Close ends a running Fetch.
//
// Deliveries are validated as pullsync.Sync validates them. A delivery that
// was not requested (or delivered twice), does not decode or is neither a
// valid CAC nor a valid SOC is a violation: the stream is reset, the peer is
// disconnected, the failure is reported to the PeerScorer and the error wraps
// ErrInvalidMessage. A delivery whose stamp the stamp validator rejects is
// dropped without penalty. Requested IDs the peer does not deliver are not
// an error. A chunk that is in the reserve when it would be stored is counted
// as a cross protocol duplicate and not stored again.
func (s *Service) Fetch(ctx context.Context, peer swarm.Address, ids []storer.ChunkID) (stored int, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel) // Close ends the fetch
	defer stop()

	for len(ids) > 0 {
		var cands []fingerprint.ID
		cands, ids, err = s.wantCandidates(ids)
		if err != nil {
			return stored, err
		}
		if len(cands) == 0 {
			continue
		}
		// wait before claiming, so that the wait cannot outlast the claims
		if _, err := s.fetchLimiter.Wait(ctx, fetchKey, len(cands)); err != nil {
			return stored, fmt.Errorf("fetch limiter: %w", err)
		}
		claimed := s.claim(peer, cands)
		if len(claimed) == 0 {
			continue
		}
		n, err := s.fetch(ctx, peer, claimed)
		stored += n
		if err != nil {
			return stored, err
		}
	}
	return stored, nil
}

// wantCandidates returns at most MaxWantIDs distinct IDs from the start of
// ids that are within the storage radius and not in the reserve, and the IDs
// not looked at yet.
func (s *Service) wantCandidates(ids []storer.ChunkID) (cands []fingerprint.ID, rest []storer.ChunkID, err error) {
	seen := make(map[fingerprint.ID]struct{})
	for len(ids) > 0 && len(cands) < MaxWantIDs {
		c := ids[0]
		ids = ids[1:]
		if !s.store.IsWithinStorageRadius(c.Address) {
			s.metrics.FetchSkipped.WithLabelValues("out_of_radius").Inc()
			continue
		}
		has, err := s.store.ReserveHasID(c.Address, c.StampHash)
		if err != nil {
			return nil, nil, fmt.Errorf("reserve has id: %w", err)
		}
		if has {
			s.metrics.FetchSkipped.WithLabelValues("held").Inc()
			continue
		}
		id := fingerprint.NewID(c.Address, c.StampHash)
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		cands = append(cands, id)
	}
	return cands, ids, nil
}

// claim claims cands for peer and returns the granted IDs.
func (s *Service) claim(peer swarm.Address, cands []fingerprint.ID) map[fingerprint.ID]struct{} {
	claimed := make(map[fingerprint.ID]struct{}, len(cands))
	for _, id := range cands {
		if r := s.wantSet.Claim(id, peer); r != wantset.Granted {
			s.metrics.FetchSkipped.WithLabelValues(r.String()).Inc()
			continue
		}
		claimed[id] = struct{}{}
	}
	return claimed
}

// releaseAll releases the claims of peer on ids.
func (s *Service) releaseAll(peer swarm.Address, ids map[fingerprint.ID]struct{}) {
	for id := range ids {
		s.wantSet.Release(id, peer)
	}
}

// fetch sends one Want of the claimed IDs to peer and handles the
// deliveries until the peer closes the stream. Every claim in claimed is
// ended before it returns.
func (s *Service) fetch(ctx context.Context, peer swarm.Address, claimed map[fingerprint.ID]struct{}) (stored int, err error) {
	// claimed holds the IDs not delivered yet
	defer s.releaseAll(peer, claimed)

	// a holder that stalls must not keep the claims past ClaimDeadline
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, s.fetchTimeout)
	defer cancel()
	stalled := func(err error) error {
		if parent.Err() == nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return s.initiatorViolation(peer, invalid("no deliveries within %s", s.fetchTimeout))
		}
		return err
	}

	want := &pb.Want{IDs: make([]*pb.ID, 0, len(claimed))}
	for id := range claimed {
		want.IDs = append(want.IDs, EncodeID(id))
	}

	stream, err := s.streamer.NewStream(ctx, peer, nil, ProtocolName, ProtocolVersion, FetchStreamName)
	if err != nil {
		return 0, fmt.Errorf("new stream: %w", err)
	}
	defer func() {
		if err != nil {
			_ = stream.Reset()
		} else {
			_ = stream.FullClose()
		}
	}()

	w, r := protobuf.NewWriter(stream), newReader(stream, maxDeliveryBytes)
	if err := w.WriteMsgWithContext(ctx, want); err != nil {
		return 0, stalled(fmt.Errorf("write want: %w", err))
	}

	for len(claimed) > 0 {
		var d pb.Delivery
		if err := readMessage(ctx, r, &d); err != nil {
			if errors.Is(err, io.EOF) {
				// the peer delivered what it holds
				return stored, nil
			}
			if errors.Is(err, ErrInvalidMessage) {
				return stored, s.initiatorViolation(peer, fmt.Errorf("delivery: %w", err))
			}
			return stored, stalled(fmt.Errorf("read delivery: %w", err))
		}
		ok, err := s.deliver(ctx, peer, claimed, &d)
		if errors.Is(err, ErrInvalidMessage) {
			return stored, s.initiatorViolation(peer, err)
		}
		if err != nil {
			return stored, err
		}
		if ok {
			stored++
		}
	}
	// every claimed ID was delivered: anything more is unsolicited
	var d pb.Delivery
	switch err := readMessage(ctx, r, &d); {
	case errors.Is(err, io.EOF):
		return stored, nil
	case err == nil || errors.Is(err, ErrInvalidMessage):
		return stored, s.initiatorViolation(peer, invalid("unsolicited delivery"))
	default:
		return stored, stalled(fmt.Errorf("read delivery: %w", err))
	}
}

// deliver validates one Delivery against the claimed IDs and stores it. The
// rules are those of pullsync.Sync (pkg/pullsync/pullsync.go), re-implemented
// here. It removes the delivered ID from claimed and ends its claim. It
// reports whether the chunk was stored; a violation wraps ErrInvalidMessage.
func (s *Service) deliver(ctx context.Context, peer swarm.Address, claimed map[fingerprint.ID]struct{}, d *pb.Delivery) (bool, error) {
	if len(d.Address) != swarm.HashSize {
		return false, invalid("delivery address of %d bytes", len(d.Address))
	}
	addr := swarm.NewAddress(d.Address)
	stamp := new(postage.Stamp)
	if err := stamp.UnmarshalBinary(d.Stamp); err != nil {
		return false, invalid("delivery stamp: %v", err)
	}
	stampHash, err := stamp.Hash()
	if err != nil {
		return false, invalid("delivery stamp hash: %v", err)
	}
	id := fingerprint.NewID(addr, stampHash)
	if _, ok := claimed[id]; !ok {
		return false, invalid("unsolicited chunk %s", addr)
	}
	delete(claimed, id)

	chunk, err := s.validStamp(swarm.NewChunk(addr, d.Data).WithStamp(stamp))
	if err != nil {
		s.wantSet.Release(id, peer)
		s.metrics.DeliveryRejected.Inc()
		s.logger.Debug("unverified stamp", "error", err, "peer_address", peer, "chunk_address", addr)
		return false, nil
	}

	if cac.Valid(chunk) {
		go s.unwrap(chunk)
	} else if sch, err := soc.FromChunkValidate(chunk); err == nil {
		s.logger.Debug("fetch gsoc", "peer_address", peer, "chunk_address", addr, "wrapped_chunk_address", sch.WrappedChunk().Address())
		s.gsocHandler(sch)
	} else {
		s.wantSet.Release(id, peer)
		return false, invalid("invalid chunk %s: %v", addr, swarm.ErrInvalidChunk)
	}

	has, err := s.store.ReserveHasID(addr, stampHash)
	if err != nil {
		s.wantSet.Release(id, peer)
		return false, fmt.Errorf("reserve has id: %w", err)
	}
	if has {
		s.metrics.CrossProtocolDuplicates.Inc()
		s.wantSet.Done(id)
		return false, nil
	}
	if err := s.store.ReservePutter().Put(ctx, chunk); err != nil {
		if errors.Is(err, storage.ErrOverwriteNewerChunk) {
			// a newer stamp of the chunk is held
			s.logger.Debug("overwrite newer chunk", "error", err, "peer_address", peer, "chunk_address", addr)
			s.wantSet.Done(id)
			return false, nil
		}
		s.wantSet.Release(id, peer)
		return false, fmt.Errorf("reserve put: %w", err)
	}
	s.wantSet.Done(id)
	s.metrics.Delivered.Inc()
	return true, nil
}

// handleFetch serves the fetch stream: it reads one Want, delivers the
// requested chunks the reserve holds, at most handleMaxChunksPerSecond per
// peer, and closes the stream. IDs it does not hold are skipped. A Want that
// does not decode, is over maxWantBytes, is empty, has more than MaxWantIDs
// IDs, an invalid ID or the same ID twice is a violation.
func (s *Service) handleFetch(ctx context.Context, p p2p.Peer, stream p2p.Stream) error {
	peer := p.Address
	release, ok := s.fetchStreams.acquire(peer)
	if !ok {
		return s.streamLimitExceeded(peer, stream, FetchStreamName, maxPeerFetchStreams)
	}
	defer release()

	w, r := protobuf.NewWriter(stream), newReader(stream, maxWantBytes)
	var want pb.Want
	if err := readMessage(ctx, r, &want); err != nil {
		if errors.Is(err, ErrInvalidMessage) {
			return s.violation(peer, stream, fmt.Errorf("want: %w", err))
		}
		_ = stream.Reset()
		return fmt.Errorf("read want: %w", err)
	}
	ids, err := parseWant(&want)
	if err != nil {
		return s.violation(peer, stream, err)
	}

	for _, id := range ids {
		ch, err := s.store.ReserveGetByID(ctx, id.Address(), id.StampHash())
		if errors.Is(err, storage.ErrNotFound) {
			continue
		}
		if err != nil {
			_ = stream.Reset()
			return fmt.Errorf("reserve get by id: %w", err)
		}
		// charged per delivered chunk: unknown IDs cost one index lookup
		if _, err := s.deliveryLimiter.Wait(ctx, peer.ByteString(), 1); err != nil {
			_ = stream.Reset()
			return fmt.Errorf("delivery limiter: %w", err)
		}
		stamp, err := ch.Stamp().MarshalBinary()
		if err != nil {
			_ = stream.Reset()
			return fmt.Errorf("marshal stamp: %w", err)
		}
		d := &pb.Delivery{Address: ch.Address().Bytes(), Data: ch.Data(), Stamp: stamp}
		if err := w.WriteMsgWithContext(ctx, d); err != nil {
			_ = stream.Reset()
			return fmt.Errorf("write delivery: %w", err)
		}
	}
	return stream.FullClose()
}

// parseWant decodes the IDs of a Want received from a peer. Errors wrap
// ErrInvalidMessage.
func parseWant(w *pb.Want) ([]fingerprint.ID, error) {
	if len(w.IDs) == 0 {
		return nil, invalid("empty want")
	}
	if len(w.IDs) > MaxWantIDs {
		return nil, invalid("want of %d ids", len(w.IDs))
	}
	ids := make([]fingerprint.ID, len(w.IDs))
	seen := make(map[fingerprint.ID]struct{}, len(w.IDs))
	for i, p := range w.IDs {
		id, err := ParseID(p)
		if err != nil {
			return nil, fmt.Errorf("want: %w", err)
		}
		if _, ok := seen[id]; ok {
			return nil, invalid("duplicate id in want")
		}
		seen[id] = struct{}{}
		ids[i] = id
	}
	return ids, nil
}
