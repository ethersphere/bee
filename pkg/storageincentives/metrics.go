// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

type metrics struct {
	// phase gauge and counter
	CurrentPhase            m.Gauge
	RevealPhase             m.Counter
	CommitPhase             m.Counter
	ClaimPhase              m.Counter
	Winner                  m.Counter
	NeighborhoodSelected    m.Counter
	SampleDuration          m.Gauge
	Round                   m.Gauge
	InsufficientFundsToPlay m.Counter

	// total calls to chain backend
	BackendCalls  m.Counter
	BackendErrors m.Counter

	// metrics for err processing
	ErrReveal         m.Counter
	ErrCommit         m.Counter
	ErrClaim          m.Counter
	ErrWinner         m.Counter
	ErrCheckIsPlaying m.Counter
}

func newMetrics() metrics {
	subsystem := "storageincentives"

	return metrics{
		CurrentPhase: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "current_phase",
			Help:      "Enum value of the current phase.",
		}),
		RevealPhase: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "reveal_phases",
			Help:      "Count of reveal phases entered.",
		}),
		CommitPhase: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "commit_phases",
			Help:      "Count of commit phases entered.",
		}),
		InsufficientFundsToPlay: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "insufficient_funds_to_play",
			Help:      "Count of games skipped due to insufficient balance to participate.",
		}),
		ClaimPhase: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "claim_phases",
			Help:      "Count of claim phases entered.",
		}),
		Winner: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "winner",
			Help:      "Count of won rounds.",
		}),
		NeighborhoodSelected: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "neighborhood_selected",
			Help:      "Count of the neighborhood being selected.",
		}),
		SampleDuration: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "reserve_sample_duration",
			Help:      "Time taken to produce a reserve sample.",
		}),
		Round: m.NewGauge(m.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "round",
			Help:      "Current round calculated from the block height.",
		}),

		// total call
		BackendCalls: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "backend_calls",
			Help:      "total chain backend calls",
		}),
		BackendErrors: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "backend_errors",
			Help:      "total chain backend errors",
		}),

		// phase errors
		ErrReveal: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "reveal_phase_errors",
			Help:      "total reveal phase errors while processing",
		}),
		ErrCommit: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "commit_phase_errors",
			Help:      "total commit phase errors while processing",
		}),
		ErrClaim: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "claim_phase_errors",
			Help:      "total claim phase errors while processing",
		}),
		ErrWinner: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "win_phase_errors",
			Help:      "total win phase while processing",
		}),
		ErrCheckIsPlaying: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "is_playing_errors",
			Help:      "total neighborhood selected errors while processing",
		}),
	}
}

// TODO: register metric
func (a *Agent) Metrics() []m.Collector {
	return m.PrometheusCollectorsFromFields(a.metrics)
}
