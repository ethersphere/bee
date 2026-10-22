// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"math"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/wantset"
	"github.com/ethersphere/bee/v2/pkg/ratelimit"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// PendingReplies reports the state kept for answers to the peer's Reply
// pages: the intervals that keep an exclusion list, the IDs in those lists,
// and all pending intervals.
func (s *Session) PendingReplies() (withExclusion, excludedIDs, intervals int) {
	for _, a := range s.answers {
		for _, iv := range a.intervals {
			if len(iv.excl) > 0 {
				withExclusion++
			}
			excludedIDs += len(iv.excl)
		}
		intervals += len(a.intervals)
	}
	return withExclusion, excludedIDs, intervals
}

// CacheCapacity returns the length of the cached prefix of the stream of r
// for a peer with Status st.
func CacheCapacity(st Status, r fingerprint.Range) uint64 { return cacheCapacity(st, r) }

// FirstBatchOf returns FirstBatch's rule with minBatch for MinBatch and the
// fraction num / den for BatchFactor: max(minBatch, ⌈num · d / den⌉ + 2).
func FirstBatchOf(d, minBatch, num, den uint64) uint64 { return firstBatch(d, minBatch, num, den) }

// DecodeCorruptions returns the value of the decode_corruption_total metric.
func (s *Service) DecodeCorruptions() float64 {
	return counterValue(s.metrics.DecodeCorruption)
}

const MaxPeerReconcileStreams = maxPeerReconcileStreams

// StatusRateLimited returns the value of the status_rate_limited_total
// metric.
func (s *Service) StatusRateLimited() float64 {
	return counterValue(s.metrics.StatusRateLimited)
}

// MappingMismatches returns the value of the mapping_mismatch_total metric.
func (s *Service) MappingMismatches() float64 {
	var m dto.Metric
	_ = s.metrics.MappingMismatch.Write(&m) // a counter's Write does not fail
	return m.GetCounter().GetValue()
}

// Local returns the Local the sessions of this node read.
func (s *Service) Local() Local {
	return s.local
}

const (
	MaxPeerFetchStreams = maxPeerFetchStreams
	MaxFetchStreamsOut  = maxFetchStreamsOut
	MaxFetchWaiters     = maxFetchWaiters
	OpenBackoffBase     = openBackoffBase
	MaxOpenBackoff      = maxOpenBackoff
)

var ErrFetchQueueFull = errFetchQueueFull

// OpenOutcome is how a fetch stream open ended.
type OpenOutcome = openOutcome

const (
	OpenSucceeded = openSucceeded
	OpenFailed    = openFailed
	OpenAborted   = openAborted
)

// OpenGate exposes the backoff of a peer's fetch stream opens.
type OpenGate struct{ g *openGate }

func NewOpenGate(base, maxBackoff time.Duration) OpenGate {
	return OpenGate{newOpenGate(base, maxBackoff)}
}

func at(t time.Time) func() time.Time { return func() time.Time { return t } }

func (g OpenGate) Allowed(peer swarm.Address, now time.Time) bool { return g.g.allowed(peer, at(now)) }

// OpenProbe is the probe an open of OpenGate.Begin made, if any (IsProbe).
type OpenProbe struct{ st *openState }

func (p OpenProbe) IsProbe() bool { return p.st != nil }

func (g OpenGate) Begin(peer swarm.Address, now time.Time) (OpenProbe, bool) {
	st, ok := g.g.begin(peer, at(now))
	return OpenProbe{st}, ok
}

func (g OpenGate) End(peer swarm.Address, now time.Time, outcome OpenOutcome, probe OpenProbe) {
	g.g.end(peer, at(now), outcome, probe.st)
}

func (g OpenGate) Clear(peer swarm.Address) { g.g.clear(peer) }

var ErrStreamLimitExceeded = errStreamLimitExceeded

// WantSet returns the want set of the service.
func (s *Service) WantSet() *wantset.Set {
	return s.wantSet
}

// SetWantSet replaces the want set of the service. It must be called before
// the service is used.
func (s *Service) SetWantSet(ws *wantset.Set) {
	s.wantSet = ws
}

// DrainFetchLimiter takes the burst of the node-wide fetch limiter, so that
// the next fetch waits for its tokens. It reports whether the burst was
// taken.
func (s *Service) DrainFetchLimiter() bool {
	return s.fetchLimiter.Allow(fetchKey, MaxWantIDs)
}

// TakeFetchTokens takes n tokens of the node-wide fetch limiter if it holds
// them, and reports whether it did.
func (s *Service) TakeFetchTokens(n int) bool {
	return s.fetchLimiter.Allow(fetchKey, n)
}

// FetchSkipped returns the fetch_skipped_total counter of reason.
func (s *Service) FetchSkipped(reason string) float64 {
	return counterValue(s.metrics.FetchSkipped.WithLabelValues(reason))
}

// WantClaims returns the want_claims counter of result.
func (s *Service) WantClaims(result string) float64 {
	return counterValue(s.metrics.WantClaims.WithLabelValues(result))
}

// RejectedCacheSize returns the value of the rejected_cache_size gauge.
func (s *Service) RejectedCacheSize() float64 {
	var m dto.Metric
	_ = s.metrics.RejectedCacheSize.Write(&m)
	return m.GetGauge().GetValue()
}

// CrossProtocolDuplicates returns the value of the
// cross_protocol_duplicates_total metric.
func (s *Service) CrossProtocolDuplicates() float64 {
	var m dto.Metric
	_ = s.metrics.CrossProtocolDuplicates.Write(&m)
	return m.GetCounter().GetValue()
}

// SessionsInUse returns the number of node-wide session tokens held.
func (s *Service) SessionsInUse() int {
	return len(s.sessions)
}

func counterValue(c interface{ Write(*dto.Metric) error }) float64 {
	var m dto.Metric
	_ = c.Write(&m)
	return m.GetCounter().GetValue()
}

// MetadataBytes returns the bytes_metadata counter of dir ("in" or "out").
func (s *Service) MetadataBytes(dir string) float64 {
	return counterValue(s.metrics.BytesMetadata.WithLabelValues(dir))
}

// ChunkBytes returns the bytes_chunks counter of dir ("in" or "out").
func (s *Service) ChunkBytes(dir string) float64 {
	return counterValue(s.metrics.BytesChunks.WithLabelValues(dir))
}

// Parts returns the parts_sent (sent) or parts_received counter of typ.
func (s *Service) Parts(sent bool, typ string) float64 {
	if sent {
		return counterValue(s.metrics.PartsSent.WithLabelValues(typ))
	}
	return counterValue(s.metrics.PartsReceived.WithLabelValues(typ))
}

func histogramValue(h interface{ Write(*dto.Metric) error }) (count uint64, sum float64) {
	var m dto.Metric
	_ = h.Write(&m)
	return m.GetHistogram().GetSampleCount(), m.GetHistogram().GetSampleSum()
}

// SessionRounds returns the sample count and sum of session_rounds.
func (s *Service) SessionRounds() (count uint64, sum float64) {
	return histogramValue(s.metrics.SessionRounds)
}

// PairwiseDiffHalf returns the sample count and sum of pairwise_diff_half.
func (s *Service) PairwiseDiffHalf() (count uint64, sum float64) {
	return histogramValue(s.metrics.PairwiseDiffHalf)
}

// PairwiseDiff returns the sample count and sum of pairwise_diff.
func (s *Service) PairwiseDiff() (count uint64, sum float64) {
	return histogramValue(s.metrics.PairwiseDiff)
}

// PairwiseDiffBuckets returns the upper bounds of the pairwise_diff buckets
// (+Inf excluded) and the cumulative sample count of each.
func (s *Service) PairwiseDiffBuckets() (bounds []float64, counts []uint64) {
	var m dto.Metric
	_ = s.metrics.PairwiseDiff.Write(&m)
	for _, b := range m.GetHistogram().GetBucket() {
		bounds = append(bounds, b.GetUpperBound())
		counts = append(counts, b.GetCumulativeCount())
	}
	return bounds, counts
}

// MoreRequests returns the value of the more_requests_total metric.
func (s *Service) MoreRequests() float64 { return counterValue(s.metrics.MoreRequests) }

// StreamsExhausted returns the streams_exhausted_total counter of level.
func (s *Service) StreamsExhausted(level string) float64 {
	return counterValue(s.metrics.StreamsExhausted.WithLabelValues(level))
}

// SymbolsPerDifference returns the sample count and sum of
// symbols_per_difference.
func (s *Service) SymbolsPerDifference() (count uint64, sum float64) {
	return histogramValue(s.metrics.SymbolsPerDifference)
}

const (
	LevelRoot   = levelRoot
	LevelLevel1 = levelLevel1
	LevelBucket = levelBucket
	LevelDeep   = levelDeep
)

// FetchTimeout returns the timeout of one Want.
func (s *Service) FetchTimeout() time.Duration { return s.fetchTimeout }

// ShortfallTolerance returns the shortfall a fetch of requested IDs
// tolerates from its peer.
func ShortfallTolerance(requested int) int { return shortfallTolerance(requested) }

// SetNow sets the clock of the fetch shortfall's rules (its windows). It
// must be called before the service is used.
func (s *Service) SetNow(now func() time.Time) { s.now = now }

const (
	ShortfallWindow    = shortfallWindow
	RootExcuseInterval = rootExcuseInterval
	BlatantMinIDs      = blatantMinIDs
	MaxMissingPerPeer  = maxMissingPerPeer
	MaxFetchResets     = maxFetchResets
	FetchResetInterval = fetchResetInterval
	ViolationGrace     = violationGrace
)

// PlausibleStep reports whether a root move from from to to excuses the IDs
// that left the root.
func PlausibleStep(from, to fingerprint.Range) bool { return plausibleStep(from, to) }

// ShortfallBook exposes the per-peer memory of the fetch shortfall.
type ShortfallBook struct{ b *shortfallBook }

func NewShortfallBook() ShortfallBook { return ShortfallBook{newShortfallBook()} }

func (b ShortfallBook) Excuse(peer swarm.Address, from, to fingerprint.Range, started, now time.Time) bool {
	return b.b.excuse(peer, from, to, started, now)
}

func (b ShortfallBook) Relisted(peer swarm.Address, ids []fingerprint.ID, now time.Time) int {
	return b.b.relisted(peer, ids, now)
}

func (b ShortfallBook) Offend(peer swarm.Address, now time.Time) bool { return b.b.offend(peer, now) }

func (b ShortfallBook) Clear(peer swarm.Address) { b.b.clear(peer) }

// DisablePeerRateLimits lifts the per-peer limits of the Statuses this node
// receives (PeerStatusInterval and PeerStatusBurst, and the drops past them)
// and of the sessions it serves (PeerSessionInterval and PeerSessionBurst).
// These limiters run on the real clock, so a simulation that runs the
// protocol on a fake clock faster than real time would trip them although
// every node keeps the rates on its own clock; the simulation checks the
// rates on its clock instead. It must be called before the service is used.
func (s *Service) DisablePeerRateLimits() {
	s.sessionLimiter = ratelimit.New(time.Nanosecond, math.MaxInt32)
	s.statusLimiter = ratelimit.New(time.Nanosecond, math.MaxInt32)
	s.statusDrops = ratelimit.New(time.Nanosecond, math.MaxInt32)
}

// DisableHints makes the service's sessions run without hint(R), so that the
// count estimate alone sizes their first symbol batches. It must be called
// before the service is used.
func (s *Service) DisableHints() { s.hints = nil }

// SessionSample is what one side of a completed session reports to
// OnSession.
type SessionSample struct {
	// Peer is the other side, and Initiator reports whether this node
	// opened the session.
	Peer      swarm.Address
	Initiator bool
	// Difference is the session's n_Δ (Session.Difference): the
	// pairwise_diff sample.
	Difference uint64
	// Exact reports whether Difference counts only the symbol streams this
	// side decoded (or nothing, for a session that found no difference): no
	// term derived from counts, so it is exact under churn too.
	Exact bool
	// Recovered counts the IDs this side's decoders recovered, both
	// directions; SymbolsReceived and SymbolsSent count the coded symbols of
	// all streams, the overshoot and the streams that did not decode
	// included.
	Recovered, SymbolsReceived, SymbolsSent uint64
}

// OnSession makes the service call f with every completed session, right
// after it observed the session's pairwise_diff sample. It must be called
// before the service is used.
func (s *Service) OnSession(f func(SessionSample)) {
	s.sessionDone = func(peer swarm.Address, sess *Session, initiator bool) {
		f(SessionSample{
			Peer:            peer,
			Initiator:       initiator,
			Difference:      sess.Difference(),
			Exact:           len(sess.listings) == 0 && len(sess.in) == 0,
			Recovered:       sess.decoded,
			SymbolsReceived: sess.symbolsReceived,
			SymbolsSent:     sess.symbolsSent,
		})
	}
}

// The per-peer fetch credit (credit.go).
const (
	CreditInitial       = creditInitial
	CreditMax           = creditMax
	CreditStoredRefund  = creditStoredRefund
	CreditPoolBurst     = creditPoolBurst
	CreditPoolInterval  = creditPoolInterval
	CreditProbeInterval = creditProbeInterval
	MaxCreditPeers      = maxCreditPeers
)

// CreditBook exposes the book of the fetch credit.
type CreditBook struct{ b *creditBook }

// NewCreditBook returns a book of at most capacity overlays whose pool is
// full at now.
func NewCreditBook(capacity int, now time.Time) CreditBook {
	b := newCreditBook(capacity)
	b.pool.AllowN(now, 0)
	return CreditBook{b}
}

func (b CreditBook) Take(peer swarm.Address, n int, now time.Time) int { return b.b.take(peer, n, now) }
func (b CreditBook) Give(peer swarm.Address, n int)                    { b.b.give(peer, n) }
func (b CreditBook) Credit(peer swarm.Address) (int, bool)             { return b.b.credit(peer) }
func (b CreditBook) Len() (peers, debts int)                           { return b.b.size() }

// DisableFetchCredit lifts the per-peer fetch credit: every request is
// granted, as before T4.7 part 4. It must be called before the service is
// used.
func (s *Service) DisableFetchCredit() { s.credits = nil }

// SetFetchCreditCapacity makes the service's credit book keep at most
// capacity overlays. It must be called before the service is used.
func (s *Service) SetFetchCreditCapacity(capacity int) { s.credits = newCreditBook(capacity) }

// FetchCreditKnown reports whether the service's credit book keeps peer.
func (s *Service) FetchCreditKnown(peer swarm.Address) bool {
	_, ok := s.credits.credit(peer)
	return ok
}

// DrainFetchCreditPool takes all of the credit pool has for new overlays
// now, as the new overlays of an attacker would.
func (s *Service) DrainFetchCreditPool() {
	now := s.now()
	s.credits.mu.Lock()
	defer s.credits.mu.Unlock()
	s.credits.pool.AllowN(now, int(s.credits.pool.TokensAt(now)))
}

// DisableDeliveryLimit lifts the per-peer rate of the chunks the fetch
// handler delivers (handleMaxChunksPerSecond), which runs on the real clock.
// It must be called before the service is used.
func (s *Service) DisableDeliveryLimit() {
	s.deliveryLimiter = ratelimit.New(time.Nanosecond, math.MaxInt32)
}
