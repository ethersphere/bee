// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"math"
	"time"

	"github.com/prometheus/client_golang/prometheus"
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

// pairwiseDiffHook is the pairwise_diff histogram that also passes each
// sample to f.
type pairwiseDiffHook struct {
	prometheus.Histogram
	f func(float64)
}

func (h pairwiseDiffHook) Observe(v float64) {
	h.f(v)
	h.Histogram.Observe(v)
}

// OnPairwiseDiff makes the service call f with every pairwise_diff sample as
// it observes it. It must be called before the service is used.
func (s *Service) OnPairwiseDiff(f func(diff float64)) {
	s.metrics.PairwiseDiff = pairwiseDiffHook{Histogram: s.metrics.PairwiseDiff, f: f}
}
