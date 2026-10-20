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
	"github.com/ethersphere/bee/v2/pkg/crypto"
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
// says nothing about the peer and needs nothing more: outside the storage
// radius, in the reserve already (found before or right after the claim, or
// at delivery: the cross protocol duplicates), not granted because the want
// set is closed, or not looked at because the fetch ended with an error.
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
	// Deferred are the IDs not requested because the want set, or the
	// peer's share of it, was full (wantset.Full, wantset.PeerFull). They
	// say nothing about the peer; request them again once claims end.
	Deferred []storer.ChunkID
	// Unverified are the IDs delivered whose stamp the stamp validator
	// could not check for a cause on this node (a batch store error, not
	// stampRejected, and the signature recovers). Their claims were
	// released; they say nothing about the peer.
	Unverified []storer.ChunkID
	// Rejected is the number of IDs rejected: delivered with a stamp the
	// stamp validator rejected (stampRejected) or older than the one the
	// reserve holds (storage.ErrOverwriteNewerChunk), or not requested
	// because they were rejected within RejectTTL. They are not requested
	// again until then.
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
// reserve holds already and the IDs the want set would not grant now
// (wantset.Set.PeekPicked: rejected within RejectTTL, held by another claim,
// or the set or the peer's share full, the Want's earlier candidates
// counted), so that they cost no fetch rate, then
// the IDs it does not grant when claimed and, once claimed, the IDs another
// fetch stored meanwhile (during the fetch limiter wait); when none remains
// no stream is opened. While a fetch holds the claim on an ID no other fetch
// requests it, so a chunk is delivered twice only after a claim expired. The
// claims of an ID end with Done when it was stored, with Reject when it was
// rejected, and with Release otherwise, on every return. The node-wide fetch
// rate (Options.FetchRate) is waited for before the IDs of each Want are
// claimed. A holder that has not finished within Options.FetchTimeout of the
// Want is a violation. Close ends a running fetch.
//
// Deliveries are validated as pullsync.Sync validates them. A delivery that
// was not requested (or delivered twice), does not decode or is neither a
// valid CAC nor a valid SOC is a violation: the stream is reset, the peer is
// disconnected, the failure is reported to the PeerScorer and the error wraps
// ErrInvalidMessage. A delivery whose stamp the stamp validator rejects
// (stampRejected), or whose stamp is older than the one the reserve holds,
// is dropped without penalty and its ID rejected for RejectTTL
// (wantset.Set.Reject); one the validator fails for another cause is
// dropped without penalty and reported Unverified. Requested
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
		cands, ids, err = s.wantCandidates(peer, ids, &res)
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
		claimed, err := s.claim(peer, cands, &res)
		if err != nil {
			return res, err
		}
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
// ids that are within the storage radius, not in the reserve and that the
// want set would grant to peer now, with the candidates before them claimed
// (PeekPicked), and the IDs not looked at yet. It records the IDs the want
// set would not grant in res.
func (s *Service) wantCandidates(peer swarm.Address, ids []storer.ChunkID, res *FetchResult) (cands []fingerprint.ID, rest []storer.ChunkID, err error) {
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
		r := s.wantSet.PeekPicked(id, peer, len(cands))
		if r == wantset.Granted {
			cands = append(cands, id)
			continue
		}
		s.metrics.WantClaims.WithLabelValues(claimLabel(r)).Inc()
		s.metrics.FetchSkipped.WithLabelValues(r.String()).Inc()
		res.add(id, r)
	}
	return cands, ids, nil
}

// add records id, which the want set did not grant with r, in res.
func (res *FetchResult) add(id fingerprint.ID, r wantset.Result) {
	switch r {
	case wantset.Held:
		res.Held = append(res.Held, chunkID(id))
	case wantset.Full, wantset.PeerFull:
		res.Deferred = append(res.Deferred, chunkID(id))
	case wantset.Rejected:
		res.Rejected++
	}
}

// claim claims cands for peer and returns the granted IDs that the reserve
// does not hold. It records the IDs held by another claim, the deferred and
// the rejected ones in res.
//
// The reserve is checked again once an ID is claimed: the candidates were
// checked before the fetch limiter wait, during which another fetch may have
// claimed, stored and ended its claim on the ID. A claim ends (Done) only
// after its chunk is stored, so an ID claimed and not in the reserve is
// requested by no other fetch until this claim ends or expires. On error the
// claims made are released.
func (s *Service) claim(peer swarm.Address, cands []fingerprint.ID, res *FetchResult) (map[fingerprint.ID]struct{}, error) {
	claimed := make(map[fingerprint.ID]struct{}, len(cands))
	for _, id := range cands {
		r := s.wantSet.Claim(id, peer)
		s.metrics.WantClaims.WithLabelValues(claimLabel(r)).Inc()
		switch r {
		case wantset.Granted:
			has, err := s.store.ReserveHasID(id.Address(), id.StampHash())
			if err != nil {
				s.wantSet.Release(id, peer)
				for c := range claimed {
					s.wantSet.Release(c, peer)
				}
				return nil, fmt.Errorf("reserve has id: %w", err)
			}
			if has {
				s.wantSet.Done(id)
				s.metrics.FetchSkipped.WithLabelValues("held").Inc()
				continue
			}
			claimed[id] = struct{}{}
			continue
		}
		res.add(id, r)
		s.metrics.FetchSkipped.WithLabelValues(r.String()).Inc()
	}
	return claimed, nil
}

// signatureRecovers reports whether the signature of stamp, for the chunk at
// addr, recovers to a signer (postage.RecoverBatchOwner).
func signatureRecovers(addr swarm.Address, stamp *postage.Stamp) bool {
	_, err := postage.RecoverBatchOwner(addr, stamp)
	return err == nil
}

// stampRejected reports whether err, an error of the stamp validator
// (postage.ValidStamp), is about the stamp itself, which the ID includes, so
// that it holds for every delivery of the ID: an unknown or expired batch
// (postage.ErrNotFound), a stamp that does not match the chunk or the batch
// (address, bucket, index, owner) or a signature of the wrong length. Other
// errors, a batch store failure among them, may pass; deliver also rejects
// a stamp whose signature does not recover (postage.RecoverBatchOwner),
// which has no sentinel.
func stampRejected(err error) bool {
	for _, target := range []error{
		postage.ErrNotFound,
		postage.ErrInvalidChunkAddress,
		postage.ErrBucketMismatch,
		postage.ErrInvalidIndex,
		postage.ErrOwnerMismatch,
		crypto.ErrInvalidLength,
		errNoStampValidator,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
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
	expired := s.claimExpired(peer, id)
	if expired {
		s.metrics.WantClaims.WithLabelValues(claimLate).Inc()
	}

	chunk, err := s.validStamp(swarm.NewChunk(addr, d.Data).WithStamp(stamp))
	if err != nil {
		s.metrics.DeliveryRejected.Inc()
		s.logger.Debug("unverified stamp", "error", err, "peer_address", peer, "chunk_address", addr)
		// A signature that does not recover is the stamp's fault too, and
		// postage.ValidStamp returns the raw recovery error for it.
		if stampRejected(err) || !signatureRecovers(addr, stamp) {
			s.wantSet.Reject(id)
			res.Rejected++
			return nil
		}
		s.wantSet.Release(id, peer)
		res.Unverified = append(res.Unverified, chunkID(id))
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
		if !expired {
			// validStamp may have waited for the batch store past the claim
			// deadline: count a claim that expired meanwhile, as after Put
			s.claimExpired(peer, id)
		}
		s.metrics.CrossProtocolDuplicates.Inc()
		s.wantSet.Done(id)
		return nil
	}
	err = s.store.ReservePutter().Put(ctx, chunk)
	if !expired {
		// Put ignores ctx and may wait for the reserve's batch lock past
		// the claim deadline: count a claim that expired meanwhile
		s.claimExpired(peer, id)
	}
	if err != nil {
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
