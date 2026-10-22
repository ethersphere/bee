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
	// four Wants of MaxWantIDs outstanding, and this node's own requests at
	// most maxFetchStreamsOut.
	maxPeerFetchStreams = 4
	// maxFetchStreamsOut is the maximum number of concurrent fetch streams
	// this node opens to one peer, whatever fetches them (round sessions,
	// stripes, retries, the wants of served sessions): half the peer's
	// limit. A stream this node reset keeps its handler on the peer until
	// the handler's next delivery fails, after its wait for the peer's
	// delivery rate, so the other half leaves room for two such handlers.
	maxFetchStreamsOut = maxPeerFetchStreams / 2
	// maxFetchWaiters bounds the fetch calls that wait for one of a peer's
	// fetch stream slots; past it a call fails at once (errFetchQueueFull).
	// Honest fetches from one peer are few: a round fetch, its retries one
	// at a time, the served sessions' fetcher, and the stripe sessions.
	maxFetchWaiters = 16

	// openBackoffBase and maxOpenBackoff set how long a peer's fetch stream
	// opens wait after one failed: openBackoffBase after the first failed
	// open, doubling with each further one up to maxOpenBackoff, until an
	// open succeeds or the peer disconnects (openGate).
	openBackoffBase = 5 * time.Second
	maxOpenBackoff  = 5 * time.Minute

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

	// skippedInReserve is the fetch_skipped_total reason of an ID the
	// reserve holds already. The want set's refusals use their own names
	// (wantset.Result.String): an ID another fetch's claim holds is "held".
	skippedInReserve = "in_reserve"
	// skippedNoCredit is the fetch_skipped_total reason of an ID not
	// requested because the peer's fetch credit is spent (ErrFetchCredit).
	skippedNoCredit = "no_credit"
)

// ErrOpenBackoff fails a fetch from a peer whose fetch stream opens back off
// after a failed one (FetchOpensBackOff), at once, without a claim. It says
// nothing about the peer: ask again once the backoff ends.
var ErrOpenBackoff = errors.New("rangesync: fetch stream opens to the peer back off")

var (
	// errNoStampValidator rejects every delivery of a Service created
	// without a stamp validator.
	errNoStampValidator = errors.New("rangesync: no stamp validator")
	// errFetchQueueFull fails a fetch from a peer that maxFetchWaiters other
	// fetch calls wait for, at once, without a claim.
	errFetchQueueFull = errors.New("rangesync: too many fetches wait for the peer")
)

// FetchResult is the outcome of FetchWithResult. Every ID passed is in at
// most one of its parts; the IDs in none were not requested for a cause that
// says nothing about the peer and needs nothing more: outside the storage
// radius, in the reserve already (found before or right after the claim, or
// at delivery: the cross protocol duplicates), not granted because the want
// set is closed, or not requested because the fetch ended with an error (a
// fetch stream that failed to open among them) other than ErrFetchCredit.
type FetchResult struct {
	// Stored is the number of chunks stored.
	Stored int
	// Undelivered are the IDs requested from the peer whose chunk was
	// neither stored nor rejected: the peer did not deliver it, delivered
	// it invalid, or its Want failed or timed out, or storing it failed on
	// this node; and the IDs not requested because they lie outside the
	// peer's root by its last Status (its radius grew since it listed
	// them). Their claims were released, or never made, so they can be
	// requested from another peer.
	Undelivered []storer.ChunkID
	// OutsideRoot is how many of Undelivered were not requested because
	// they lie outside the peer's root: the peer was not asked for them.
	OutsideRoot int
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
	// NoCredit are the IDs not requested because the peer's fetch credit
	// was spent (ErrFetchCredit, credit.go): those of the Want that found it
	// spent and those not looked at yet. Request them from other peers.
	NoCredit []storer.ChunkID
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
// counted), so that they cost no fetch rate; when none remains no stream is
// opened. Then it takes one of the fetch streams this node opens to peer (at
// most maxFetchStreamsOut at once, half the peer's own limit, whatever runs
// the fetches), takes the peer's fetch credit for the IDs (credit.go: the
// IDs it does not cover wait for the next Want, and with none left the fetch
// requests nothing more and fails with ErrFetchCredit, its IDs in
// FetchResult.NoCredit), opens the stream within Options.FetchTimeout, waits
// for the node-wide fetch rate (Options.FetchRate) with the stream open, and
// claims the IDs. It drops the IDs it is not granted and, once claimed, the
// IDs another fetch stored meanwhile (during the fetch limiter wait); when
// none remains it ends the stream without a Want. While a fetch holds the claim
// on an ID no other fetch requests it, so a chunk is delivered twice only
// after a claim expired. The claims of an ID end with Done when it was
// stored, with Reject when it was rejected, and with Release otherwise, on
// every return. A holder that has not finished within Options.FetchTimeout
// of the claims is a violation. Close ends a running fetch.
//
// Deliveries are validated as pullsync.Sync validates them. A delivery that
// was not requested (or delivered twice), does not decode or is neither a
// valid CAC nor a valid SOC is a violation: the stream is reset, the peer is
// disconnected, the failure is reported to the PeerScorer and the error wraps
// ErrInvalidMessage. A delivery whose stamp the stamp validator rejects
// (stampRejected), or whose stamp is older than the one the reserve holds,
// is dropped and its ID rejected for RejectTTL (wantset.Set.RejectFrom,
// against the peer's share of the rejected cache); one the validator fails
// for another cause is dropped and reported Unverified. A chunk that is in
// the reserve when it would be stored is counted as a cross protocol
// duplicate and not stored again.
//
// The peer listed the IDs it is asked for, so what it does not deliver
// counts against it (§9, bogus wants; the rules are in shortfall.go): the
// IDs requested in Wants it closed without delivering them, and its
// deliveries with a stamp invalid on every node (stampInvalid). A blatant
// shortfall, judged after each Want, ends the fetch as a violation. At the
// end, a shortfall of IDs the peer left missing before, or of invalid
// stamps, above shortfallTolerance of the IDs requested is an offense: the
// first is scored without a disconnect and the error wraps
// ErrFetchShortfall, a repeat is a violation. Held, deferred and unverified
// IDs, rejections that depend on the chain state or on this node's reserve,
// and the IDs of a Want that failed do not count. No ID outside the peer's
// root by its last Status is requested. A fetch stream that fails with IDs
// undelivered (the peer reset it) is a violation past maxFetchResets, then
// one per fetchResetInterval, per peer. A fetch stream that fails to open is
// never charged: it took no fetch rate and holds no claim, and the peer's
// credit for it goes back. It backs off the peer's opens instead (openGate):
// until the backoff ends, the peer's fetch calls fail at once, unscored and
// without a claim, and then one open probes the peer. A call also fails at
// once while maxFetchWaiters others wait for the peer's stream slots. Apart
// from all these, what the peer is asked for and does not deliver spends its
// fetch credit (credit.go), which only the chunks it delivers refill.
func (s *Service) FetchWithResult(ctx context.Context, peer swarm.Address, ids []storer.ChunkID) (FetchResult, error) {
	return s.FetchWithProgress(ctx, peer, ids, nil)
}

// FetchWithProgress is FetchWithResult that also reports each chunk it
// stores as it stores it: stored, if not nil, is called once per chunk
// counted in FetchResult.Stored, right after the chunk was stored, on the
// fetch's goroutine. It must not block, and must not call the Service.
func (s *Service) FetchWithProgress(ctx context.Context, peer swarm.Address, ids []storer.ChunkID, stored func()) (res FetchResult, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel) // Close ends the fetch
	defer stop()

	sf := shortfall{startedAt: s.now()}
	if st, ok := s.LastStatus(peer); ok {
		sf.start, sf.hasStart = st.Root, true
	}
	defer func() {
		if errors.Is(err, ErrInvalidMessage) {
			return // reported already: one score per fetch
		}
		if verr := s.judgeShortfall(peer, &sf, true); verr != nil {
			err = verr
		}
	}()
	for len(ids) > 0 {
		var cands []fingerprint.ID
		cands, ids, err = s.wantCandidates(peer, ids, &res)
		if err != nil {
			return res, err
		}
		if len(cands) == 0 {
			continue
		}
		rest, err := s.want(ctx, peer, cands, &res, &sf, stored)
		if errors.Is(err, ErrFetchCredit) {
			res.NoCredit = append(chunkIDs(rest), ids...)
			s.metrics.FetchSkipped.WithLabelValues(skippedNoCredit).Add(float64(len(res.NoCredit)))
			return res, err
		}
		if err != nil {
			return res, err
		}
		// the candidates the credit did not cover go first in the next Want
		ids = append(chunkIDs(rest), ids...)
		// a blatant shortfall ends the fetch now
		if err := s.judgeShortfall(peer, &sf, false); err != nil {
			return res, err
		}
	}
	return res, nil
}

// want requests cands from peer in one Want: it takes one of the peer's
// fetch stream slots (maxFetchStreamsOut), takes the peer's fetch credit for
// cands (credit.go), opens the stream, waits for the fetch rate, claims the
// candidates the credit covers and fetches the claimed IDs on the stream,
// recording the peer's shortfall in sf and calling stored, if not nil, for
// each chunk stored. It returns the candidates the credit did not cover
// (rest), which the caller requests in its next Want, once this one settled
// the credit; with no credit left it requests nothing and fails with
// ErrFetchCredit, rest holding every candidate. The credit is taken with the
// slot, so that at most maxFetchStreamsOut Wants hold a peer's credit at
// once, and before the open and the fetch rate, so that a peer with none
// left costs neither. The credit of the candidates not requested in the end
// (the open failed, the claims were not granted) goes back to the peer. The
// open comes before the fetch rate and the claims, so that an open that
// fails or stalls spends neither and is not charged to the peer; the waits
// come before the claims, so that they cannot outlast them. The slot is
// held from the open to the stream's end. While the peer's opens back off
// (openGate), it fails at once, before and after the wait for the slot.
func (s *Service) want(ctx context.Context, peer swarm.Address, cands []fingerprint.ID, res *FetchResult, sf *shortfall, stored func()) (rest []fingerprint.ID, err error) {
	if !s.opens.allowed(peer, s.now) {
		return nil, ErrOpenBackoff
	}
	release, err := s.fetchStreamsOut.acquire(ctx, peer)
	if err != nil {
		return nil, fmt.Errorf("fetch stream slot: %w", err)
	}
	defer release()
	granted := s.credits.take(peer, len(cands), s.now())
	if granted == 0 {
		return cands, ErrFetchCredit
	}
	cands, rest = cands[:granted], cands[granted:]
	unspent := granted // the credit of the candidates not requested
	defer func() { s.credits.give(peer, unspent) }()
	stream, err := s.openFetchStream(ctx, peer)
	if err != nil {
		return rest, err
	}
	if _, err := s.fetchLimiter.Wait(ctx, fetchKey, len(cands)); err != nil {
		_ = stream.Reset()
		return rest, fmt.Errorf("fetch limiter: %w", err)
	}
	claimed, err := s.claim(peer, cands, res)
	if err != nil {
		_ = stream.Reset()
		return rest, err
	}
	unspent -= len(claimed) // fetch settles the credit of the claimed IDs
	if len(claimed) == 0 {
		// nothing left to ask for: end the stream without a Want, which the
		// peer's handler does not hold against this node
		_ = stream.Reset()
		return rest, nil
	}
	return rest, s.fetch(ctx, peer, stream, claimed, res, sf, stored)
}

// FetchOpensBackOff reports whether peer's fetch stream opens back off now
// (openGate): its last open failed and the backoff has not ended, or an open
// probes it after the backoff. A fetch from the peer then fails at once with
// ErrOpenBackoff.
func (s *Service) FetchOpensBackOff(peer swarm.Address) bool {
	return !s.opens.allowed(peer, s.now)
}

// openFetchStream opens a fetch stream to peer within fetchTimeout, through
// the peer's open gate (openGate): it fails at once while the peer's opens
// back off, and a failed open starts or extends the backoff. libp2p's
// NewStream obeys only the context in its wait for identify, its protocol
// negotiation and the headers exchange (HeadersRWTimeout bounds only the
// last), so the timeout bounds the whole open. A failed open is not charged
// to the peer: the caller has taken no fetch rate and claimed nothing yet.
// An open this node ends (ctx) or that finds the peer gone
// (p2p.ErrPeerNotFound) does not back off. Nor does a failure recorded for
// a peer whose Status is gone meanwhile: it disconnected, and the
// disconnect hook may have cleared its gate before.
func (s *Service) openFetchStream(ctx context.Context, peer swarm.Address) (p2p.Stream, error) {
	probe, ok := s.opens.begin(peer, s.now)
	if !ok {
		return nil, ErrOpenBackoff
	}
	octx, cancel := context.WithTimeout(ctx, s.fetchTimeout)
	defer cancel()
	stream, err := s.streamer.NewStream(octx, peer, nil, ProtocolName, ProtocolVersion, FetchStreamName)
	switch {
	case err == nil:
		s.opens.end(peer, s.now, openSucceeded, probe)
		return stream, nil
	case ctx.Err() != nil, errors.Is(err, p2p.ErrPeerNotFound):
		s.opens.end(peer, s.now, openAborted, probe)
	default:
		s.opens.end(peer, s.now, openFailed, probe)
		if _, ok := s.LastStatus(peer); !ok {
			s.opens.clear(peer)
		}
	}
	return nil, fmt.Errorf("new stream: %w", err)
}

// wantCandidates returns at most MaxWantIDs distinct IDs from the start of
// ids that are within the storage radius, not in the reserve, inside the
// peer's root by its last Status and that the want set would grant to peer
// now, with the candidates before them claimed (PeekPicked), and the IDs not
// looked at yet. It records the IDs outside the peer's root as Undelivered
// and those the want set would not grant in res.
func (s *Service) wantCandidates(peer swarm.Address, ids []storer.ChunkID, res *FetchResult) (cands []fingerprint.ID, rest []storer.ChunkID, err error) {
	remote, hasRoot := s.LastStatus(peer)
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
			return nil, nil, fmt.Errorf("%w: reserve has id: %w", ErrLocal, err)
		}
		if has {
			s.metrics.FetchSkipped.WithLabelValues(skippedInReserve).Inc()
			continue
		}
		id := fingerprint.NewID(c.Address, c.StampHash)
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if hasRoot && !remote.Root.ContainsID(id) {
			// the peer's root moved away from it since it listed it
			s.metrics.FetchSkipped.WithLabelValues("outside_peer_root").Inc()
			res.Undelivered = append(res.Undelivered, chunkID(id))
			res.OutsideRoot++
			continue
		}
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
				return nil, fmt.Errorf("%w: reserve has id: %w", ErrLocal, err)
			}
			if has {
				s.wantSet.Done(id)
				s.metrics.FetchSkipped.WithLabelValues(skippedInReserve).Inc()
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

// stampInvalid reports whether a stamp the stamp validator failed with err
// (recovers: whether its signature recovers, signatureRecovers) is invalid on
// every node, whatever chain state it has seen: a signature that does not
// recover or has the wrong length, or a chunk address, bucket or owner that
// does not match it. An honest peer validated the stamp when it stored the
// chunk, so delivering such a stamp is the peer's fault. An unknown or
// expired batch (postage.ErrNotFound) and an index past the batch depth
// (postage.ErrInvalidIndex, which a dilution raises) depend on the chain
// state each node has seen, and errNoStampValidator on this node.
func stampInvalid(err error, recovers bool) bool {
	if errors.Is(err, errNoStampValidator) {
		return false
	}
	if !recovers {
		return true
	}
	for _, target := range []error{
		postage.ErrInvalidChunkAddress,
		postage.ErrBucketMismatch,
		postage.ErrOwnerMismatch,
		crypto.ErrInvalidLength,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
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

// chainStateRejected reports whether err, an error of the stamp validator for
// a stamp whose signature recovers, depends on the chain state this node has
// seen: an unknown or expired batch (postage.ErrNotFound), or an index past
// the batch depth (postage.ErrInvalidIndex), which a dilution this node has
// not seen yet raises. An honest peer that has seen another chain state (a
// batch that expired here first, a dilution or a new batch not seen here
// yet) delivers such stamps, and so does a peer that makes them up: the
// batch store forgets an expired batch, and the index is checked before the
// stamp's owner.
func chainStateRejected(err error) bool {
	return errors.Is(err, postage.ErrNotFound) || errors.Is(err, postage.ErrInvalidIndex)
}

// excuseChainState gives back the unit of fetch credit of a delivery of id
// by peer that this node rejected for the chain state it has seen
// (chainStateRejected) when this node held id itself, until it removed it
// from its reserve (removals: the batch expired here first), and else when
// the peer has an excuse left, which it spends (credit.go). Each removal
// excuses one delivery.
func (s *Service) excuseChainState(peer swarm.Address, id fingerprint.ID) {
	if s.removals.take(id) {
		s.credits.give(peer, 1)
		return
	}
	s.credits.excuse(peer)
}

// chunkID returns the storer.ChunkID of id, without a batch ID.
func chunkID(id fingerprint.ID) storer.ChunkID {
	return storer.ChunkID{Address: id.Address(), StampHash: id.StampHash()}
}

// chunkIDs returns the storer.ChunkIDs of ids, without batch IDs.
func chunkIDs(ids []fingerprint.ID) []storer.ChunkID {
	out := make([]storer.ChunkID, len(ids))
	for i, id := range ids {
		out[i] = chunkID(id)
	}
	return out
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

// fetch sends one Want of the claimed IDs to peer on stream, which it ends,
// and handles the deliveries until the peer closes the stream, recording the
// peer's shortfall in sf: the Want's IDs as requested, those still
// undelivered when the peer closed the stream as missing, and the stamps
// invalid on every node. It calls stored, if not nil, for each chunk stored
// (deliver). A holder that has not finished within fetchTimeout
// of the claims is a violation, judged only while the peer has a Status
// (the rules judge no peer that disconnected). A stream that fails with
// claimed IDs undelivered takes one of the peer's fetch resets
// (chargeReset), past which it is a violation; this node ending the fetch
// does not. Every claim in claimed is ended before it returns. It settles
// the peer's fetch credit of the claimed IDs (credit.go): deliver gives back
// what each delivery earns, and the IDs still undelivered get their credit
// back only when this node ended the Want (its context, Close, a local
// error).
func (s *Service) fetch(ctx context.Context, peer swarm.Address, stream p2p.Stream, claimed map[fingerprint.ID]struct{}, res *FetchResult, sf *shortfall, stored func()) (err error) {
	parent := ctx
	// claimed holds the IDs not delivered yet
	defer func() {
		if parent.Err() != nil || errors.Is(err, ErrLocal) {
			s.credits.give(peer, len(claimed)) // not the peer's doing
		}
		for id := range claimed {
			s.claimExpired(peer, id)
			s.undelivered(peer, id, res)
		}
	}()

	// a holder that stalls must not keep the claims past ClaimDeadline
	ctx, cancel := context.WithTimeout(ctx, s.fetchTimeout)
	defer cancel()
	stalled := func(err error) error {
		switch {
		case parent.Err() != nil:
			return err // this node ended the fetch
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			if _, ok := s.LastStatus(peer); !ok {
				return err // the peer disconnected: not judged
			}
			return s.initiatorViolation(peer, invalid("no deliveries within %s", s.fetchTimeout))
		case len(claimed) > 0 && s.chargeReset(peer):
			return s.initiatorViolation(peer, invalid("fetch streams fail with ids undelivered, past %d: %v", maxFetchResets, err))
		}
		return err
	}

	want := &pb.Want{IDs: make([]*pb.ID, 0, len(claimed))}
	for id := range claimed {
		want.IDs = append(want.IDs, EncodeID(id))
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
	sf.requested += len(want.IDs)

	for len(claimed) > 0 {
		var d pb.Delivery
		if err := readMessage(ctx, r, &d); err != nil {
			if errors.Is(err, io.EOF) {
				// the peer delivered what it holds: it listed the rest
				for id := range claimed {
					sf.missing = append(sf.missing, id)
				}
				return nil
			}
			if errors.Is(err, ErrInvalidMessage) {
				return s.initiatorViolation(peer, fmt.Errorf("delivery: %w", err))
			}
			return stalled(fmt.Errorf("read delivery: %w", err))
		}
		s.metrics.BytesChunks.WithLabelValues(dirIn).Add(float64(wireSize(&d)))
		err := s.deliver(ctx, peer, claimed, &d, res, sf, stored)
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
// the outcome in res, and a stamp invalid on every node in sf. A chunk it
// stores is reported to stored, if not nil. A delivery whose claim had
// expired is counted late (want_claims). A violation wraps ErrInvalidMessage.
// It gives the peer the fetch credit the delivery earns (credit.go): a stored
// chunk creditStoredRefund and an excuse; a valid chunk the reserve holds
// already or one older than the reserve's, a stamp that could not be checked
// for a cause on this node and a local error one; a stamp rejected for the
// chain state this node has seen one if this node held the ID itself or the
// peer spends an excuse (excuseChainState); another rejected stamp or chunk
// nothing.
func (s *Service) deliver(ctx context.Context, peer swarm.Address, claimed map[fingerprint.ID]struct{}, d *pb.Delivery, res *FetchResult, sf *shortfall, stored func()) error {
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
		recovers := signatureRecovers(addr, stamp)
		if stampInvalid(err, recovers) {
			sf.invalid++
		}
		if stampRejected(err) || !recovers {
			s.wantSet.RejectFrom(id, peer)
			res.Rejected++
			if recovers && chainStateRejected(err) {
				s.excuseChainState(peer, id)
			}
			return nil
		}
		s.wantSet.Release(id, peer)
		res.Unverified = append(res.Unverified, chunkID(id))
		s.credits.give(peer, 1)
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
		s.credits.give(peer, 1)
		return fmt.Errorf("%w: reserve has id: %w", ErrLocal, err)
	}
	if has {
		if !expired {
			// validStamp may have waited for the batch store past the claim
			// deadline: count a claim that expired meanwhile, as after Put
			s.claimExpired(peer, id)
		}
		s.metrics.CrossProtocolDuplicates.Inc()
		s.wantSet.Done(id)
		s.credits.give(peer, 1)
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
			// again for RejectTTL. Neutral: a peer that holds the older
			// one delivers it honestly (§7.7)
			s.logger.Debug("overwrite newer chunk", "error", err, "peer_address", peer, "chunk_address", addr)
			s.wantSet.RejectFrom(id, peer)
			res.Rejected++
			s.credits.give(peer, 1)
			return nil
		}
		s.undelivered(peer, id, res)
		s.credits.give(peer, 1)
		return fmt.Errorf("%w: reserve put: %w", ErrLocal, err)
	}
	s.wantSet.Done(id)
	s.metrics.Delivered.Inc()
	res.Stored++
	s.credits.stored(peer)
	if stored != nil {
		stored()
	}
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
