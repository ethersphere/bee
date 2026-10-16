// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package rangesync

import (
	"github.com/ethersphere/bee/v2/pkg/rangesync/wantset"
	dto "github.com/prometheus/client_model/go"
)

// PendingReplies reports the state kept for answers to the peer's Reply
// pages: the intervals that keep an exclusion list, the IDs in those lists,
// and all pending intervals.
func (s *Session) PendingReplies() (withExclusion, excludedIDs, intervals int) {
	return s.pendingReplies()
}

const MaxPeerReconcileStreams = maxPeerReconcileStreams

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

const MaxPeerFetchStreams = maxPeerFetchStreams

var ErrStreamLimitExceeded = errStreamLimitExceeded

// WantSet returns the want set of the service.
func (s *Service) WantSet() *wantset.Set {
	return s.wantSet
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
