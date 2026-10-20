// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/ethersphere/bee/v2/pkg/rangesync/fingerprint"
	"github.com/ethersphere/bee/v2/pkg/rangesync/wantset"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// PendingReplies reports the state kept for answers to the peer's Reply
// pages: the intervals that keep an exclusion list, the IDs in those lists,
// and all pending intervals.
func (s *Session) PendingReplies() (withExclusion, excludedIDs, intervals int) {
	return s.pendingReplies()
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
)

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

// PairwiseDiff returns the sample count and sum of pairwise_diff_half.
func (s *Service) PairwiseDiff() (count uint64, sum float64) {
	return histogramValue(s.metrics.PairwiseDiff)
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
)

// PlausibleStep reports whether a root move from from to to excuses the IDs
// that left the root.
func PlausibleStep(from, to fingerprint.Range) bool { return plausibleStep(from, to) }

// ShortfallBook exposes the per-peer memory of the fetch shortfall.
type ShortfallBook struct{ b *shortfallBook }

func NewShortfallBook() ShortfallBook { return ShortfallBook{newShortfallBook()} }

func (b ShortfallBook) Excuse(peer swarm.Address, from, to fingerprint.Range, now time.Time) bool {
	return b.b.excuse(peer, from, to, now)
}

func (b ShortfallBook) Relisted(peer swarm.Address, ids []fingerprint.ID, now time.Time) int {
	return b.b.relisted(peer, ids, now)
}

func (b ShortfallBook) Offend(peer swarm.Address, now time.Time) bool { return b.b.offend(peer, now) }

func (b ShortfallBook) Clear(peer swarm.Address) { b.b.clear(peer) }
