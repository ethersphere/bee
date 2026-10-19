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

// FetchResult is the outcome of FetchWithResult. Every ID passed is in at
// most one of its parts; the IDs in none were not requested for a cause that
// says nothing about the peer: outside the storage radius, in the reserve
// already (a delivery included, the cross protocol duplicates), not granted
// because the want set or the peer's share of it is full, or not looked at
// because the fetch ended with an error.
type FetchResult struct {
	// Stored is the number of chunks stored.
	Stored int
	// Undelivered are the IDs requested from the peer whose chunk was
	// neither stored nor rejected: the peer did not deliver it, delivered
	// it invalid, or its Want failed or timed out, or storing it failed on
	// this node. Their claims were released, so they can be requested from
	// another peer.
	Undelivered []storer.ChunkID
	// Held are the IDs not requested because another fetch held their
	// claim (wantset.Held), through another peer or this one. The peer is
	// another source of them should that claim end without the chunk.
	Held []storer.ChunkID
	// Rejected is the number of IDs rejected: delivered with a stamp the
	// stamp validator rejected or older than the one the reserve holds
	// (storage.ErrOverwriteNewerChunk), or not requested because they were
	// rejected within RejectTTL. They are not requested again until then.
	Rejected int
}

// Fetch is FetchWithResult, returning only the number of chunks stored.
func (s *Service) Fetch(ctx context.Context, peer swarm.Address, ids []storer.ChunkID) (stored int, err error) {
	res, err := s.FetchWithResult(ctx, peer, ids)
	return res.Stored, err
}

// FetchWithResult requests the chunks with the given IDs (batch IDs are
// ignored) from peer on fetch streams, MaxWantIDs per Want, stores the valid
// deliveries in the reserve and reports the outcome; on error, the outcome
// so far.
//
// Before each Want it drops the IDs outside the storage radius, the IDs the
// reserve holds already, the IDs rejected within RejectTTL and the IDs the
// want set does not grant; when none remains no stream is opened. The claims
// of an ID end with Done when it was stored, with Reject when it was
// rejected, and with Release otherwise, on every return. The node-wide fetch
// rate (Options.FetchRate) is waited for before the IDs of each Want are
// claimed. A holder that has not finished within Options.FetchTimeout of the
// Want is a violation. Close ends a running fetch.
//
// Deliveries are validated as pullsync.Sync validates them. A delivery that
// was not requested (or delivered twice), does not decode or is neither a
// valid CAC nor a valid SOC is a violation: the stream is reset, the peer is
// disconnected, the failure is reported to the PeerScorer and the error wraps
// ErrInvalidMessage. A delivery whose stamp the stamp validator rejects, or
// whose stamp is older than the one the reserve holds, is dropped without
// penalty and its ID rejected for RejectTTL (wantset.Set.Reject). Requested
// IDs the peer does not deliver are not an error. A chunk that is in the
// reserve when it would be stored is counted as a cross protocol duplicate
// and not stored again.
func (s *Service) FetchWithResult(ctx context.Context, peer swarm.Address, ids []storer.ChunkID) (res FetchResult, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel) // Close ends the fetch
	defer stop()

	for len(ids) > 0 {
		var cands []fingerprint.ID
		cands, ids, err = s.wantCandidates(ids, &res)
		if err != nil {
			return res, err
		}
		if len(cands) == 0 {
			continue
		}
		// wait before claiming, so that the wait cannot outlast the claims
		if _, err := s.fetchLimiter.Wait(ctx, fetchKey, len(cands)); err != nil {
			return res, fmt.Errorf("fetch limiter: %w", err)
		}
		claimed := s.claim(peer, cands, &res)
		if len(claimed) == 0 {
			continue
		}
		if err := s.fetch(ctx, peer, claimed, &res); err != nil {
			return res, err
		}
	}
	return res, nil
}

// wantCandidates returns at most MaxWantIDs distinct IDs from the start of
// ids that are within the storage radius, not in the reserve and not
// rejected, and the IDs not looked at yet. It counts the rejected ones in
// res.
func (s *Service) wantCandidates(ids []storer.ChunkID, res *FetchResult) (cands []fingerprint.ID, rest []storer.ChunkID, err error) {
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
		if s.wantSet.IsRejected(id) {
			s.metrics.FetchSkipped.WithLabelValues(claimRejected).Inc()
			s.metrics.WantClaims.WithLabelValues(claimRejected).Inc()
			res.Rejected++
			continue
		}
		cands = append(cands, id)
	}
	return cands, ids, nil
}

// claim claims cands for peer and returns the granted IDs. It records the IDs
// held by another claim and the rejected ones in res.
func (s *Service) claim(peer swarm.Address, cands []fingerprint.ID, res *FetchResult) map[fingerprint.ID]struct{} {
	claimed := make(map[fingerprint.ID]struct{}, len(cands))
	for _, id := range cands {
		r := s.wantSet.Claim(id, peer)
		s.metrics.WantClaims.WithLabelValues(claimLabel(r)).Inc()
		switch r {
		case wantset.Granted:
			claimed[id] = struct{}{}
			continue
		case wantset.Held:
			res.Held = append(res.Held, chunkID(id))
		case wantset.Rejected:
			res.Rejected++
		}
		s.metrics.FetchSkipped.WithLabelValues(r.String()).Inc()
	}
	return claimed
}

// chunkID returns the storer.ChunkID of id, without a batch ID.
func chunkID(id fingerprint.ID) storer.ChunkID {
	return storer.ChunkID{Address: id.Address(), StampHash: id.StampHash()}
}

// claimExpired reports whether the claim of peer on id, granted to this
// fetch, has ended without this fetch ending it, and counts it as expired.
// Only an expiry ends a granted claim before its fetch does, or, after one, a
// fetch through another peer that stored or rejected the ID. A closed
// service is not counted: Close drops every claim.
func (s *Service) claimExpired(peer swarm.Address, id fingerprint.ID) bool {
	if s.wantSet.HeldBy(id, peer) || s.ctx.Err() != nil {
		return false
	}
	s.metrics.WantClaims.WithLabelValues(claimExpired).Inc()
	return true
}

// undelivered releases the claim of peer on id, whose chunk was not stored,
// and records id in res as undelivered.
func (s *Service) undelivered(peer swarm.Address, id fingerprint.ID, res *FetchResult) {
	s.wantSet.Release(id, peer)
	res.Undelivered = append(res.Undelivered, chunkID(id))
}

// fetch sends one Want of the claimed IDs to peer and handles the
// deliveries until the peer closes the stream. Every claim in claimed is
// ended before it returns.
func (s *Service) fetch(ctx context.Context, peer swarm.Address, claimed map[fingerprint.ID]struct{}, res *FetchResult) (err error) {
	// claimed holds the IDs not delivered yet
	defer func() {
		for id := range claimed {
			s.claimExpired(peer, id)
			s.undelivered(peer, id, res)
		}
	}()

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
		return fmt.Errorf("new stream: %w", err)
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
		return stalled(fmt.Errorf("write want: %w", err))
	}
	s.metrics.BytesMetadata.WithLabelValues(dirOut).Add(float64(wireSize(want)))

	for len(claimed) > 0 {
		var d pb.Delivery
		if err := readMessage(ctx, r, &d); err != nil {
			if errors.Is(err, io.EOF) {
				// the peer delivered what it holds
				return nil
			}
			if errors.Is(err, ErrInvalidMessage) {
				return s.initiatorViolation(peer, fmt.Errorf("delivery: %w", err))
			}
			return stalled(fmt.Errorf("read delivery: %w", err))
		}
		s.metrics.BytesChunks.WithLabelValues(dirIn).Add(float64(wireSize(&d)))
		err := s.deliver(ctx, peer, claimed, &d, res)
		if errors.Is(err, ErrInvalidMessage) {
			return s.initiatorViolation(peer, err)
		}
		if err != nil {
			return err
		}
	}
	// every claimed ID was delivered: anything more is unsolicited
	var d pb.Delivery
	switch err := readMessage(ctx, r, &d); {
	case errors.Is(err, io.EOF):
		return nil
	case err == nil || errors.Is(err, ErrInvalidMessage):
		return s.initiatorViolation(peer, invalid("unsolicited delivery"))
	default:
		return stalled(fmt.Errorf("read delivery: %w", err))
	}
}

// deliver validates one Delivery against the claimed IDs and stores it. The
// rules are those of pullsync.Sync (pkg/pullsync/pullsync.go), re-implemented
// here. It removes the delivered ID from claimed, ends its claim and records
// the outcome in res. A delivery whose claim had expired is counted late
// (want_claims). A violation wraps ErrInvalidMessage.
func (s *Service) deliver(ctx context.Context, peer swarm.Address, claimed map[fingerprint.ID]struct{}, d *pb.Delivery, res *FetchResult) error {
	if len(d.Address) != swarm.HashSize {
		return invalid("delivery address of %d bytes", len(d.Address))
	}
	addr := swarm.NewAddress(d.Address)
	stamp := new(postage.Stamp)
	if err := stamp.UnmarshalBinary(d.Stamp); err != nil {
		return invalid("delivery stamp: %v", err)
	}
	stampHash, err := stamp.Hash()
	if err != nil {
		return invalid("delivery stamp hash: %v", err)
	}
	id := fingerprint.NewID(addr, stampHash)
	if _, ok := claimed[id]; !ok {
		return invalid("unsolicited chunk %s", addr)
	}
	delete(claimed, id)
	if s.claimExpired(peer, id) {
		s.metrics.WantClaims.WithLabelValues(claimLate).Inc()
	}

	chunk, err := s.validStamp(swarm.NewChunk(addr, d.Data).WithStamp(stamp))
	if err != nil {
		s.wantSet.Reject(id)
		res.Rejected++
		s.metrics.DeliveryRejected.Inc()
		s.logger.Debug("unverified stamp", "error", err, "peer_address", peer, "chunk_address", addr)
		return nil
	}

	if cac.Valid(chunk) {
		go s.unwrap(chunk)
	} else if sch, err := soc.FromChunkValidate(chunk); err == nil {
		s.logger.Debug("fetch gsoc", "peer_address", peer, "chunk_address", addr, "wrapped_chunk_address", sch.WrappedChunk().Address())
		s.gsocHandler(sch)
	} else {
		s.undelivered(peer, id, res)
		return invalid("invalid chunk %s: %v", addr, swarm.ErrInvalidChunk)
	}

	has, err := s.store.ReserveHasID(addr, stampHash)
	if err != nil {
		s.undelivered(peer, id, res)
		return fmt.Errorf("reserve has id: %w", err)
	}
	if has {
		s.metrics.CrossProtocolDuplicates.Inc()
		s.wantSet.Done(id)
		return nil
	}
	if err := s.store.ReservePutter().Put(ctx, chunk); err != nil {
		if errors.Is(err, storage.ErrOverwriteNewerChunk) {
			// a newer stamp of the chunk is held: do not fetch this one
			// again for RejectTTL
			s.logger.Debug("overwrite newer chunk", "error", err, "peer_address", peer, "chunk_address", addr)
			s.wantSet.Reject(id)
			res.Rejected++
			return nil
		}
		s.undelivered(peer, id, res)
		return fmt.Errorf("reserve put: %w", err)
	}
	s.wantSet.Done(id)
	s.metrics.Delivered.Inc()
	res.Stored++
	return nil
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
	s.metrics.BytesMetadata.WithLabelValues(dirIn).Add(float64(wireSize(&want)))
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
		s.metrics.BytesChunks.WithLabelValues(dirOut).Add(float64(wireSize(d)))
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
