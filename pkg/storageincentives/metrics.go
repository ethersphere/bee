// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	// phase gauge and counter
	CurrentPhase            prometheus.Gauge
	RevealPhase             prometheus.Counter
	CommitPhase             prometheus.Counter
	ClaimPhase              prometheus.Counter
	Winner                  prometheus.Counter
	NeighborhoodSelected    prometheus.Counter
	SampleDuration          prometheus.Gauge
	Round                   prometheus.Gauge
	InsufficientFundsToPlay prometheus.Counter

	// total calls to chain backend
	BackendCalls  prometheus.Counter
	BackendErrors prometheus.Counter

	// metrics for err processing
	ErrReveal         prometheus.Counter
	ErrCommit         prometheus.Counter
	ErrClaim          prometheus.Counter
	ErrWinner         prometheus.Counter
	ErrCheckIsPlaying prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "storageincentives"

	return metrics{
		CurrentPhase: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "current_phase",
			Help:      "Enum value of the current phase.",
		}),
		RevealPhase: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "reveal_phases",
			Help:      "Count of reveal phases entered.",
		}),
		CommitPhase: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "commit_phases",
			Help:      "Count of commit phases entered.",
		}),
		InsufficientFundsToPlay: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "insufficient_funds_to_play",
			Help:      "Count of games skipped due to insufficient balance to participate.",
		}),
		ClaimPhase: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "claim_phases",
			Help:      "Count of claim phases entered.",
		}),
		Winner: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "winner",
			Help:      "Count of won rounds.",
		}),
		NeighborhoodSelected: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "neighborhood_selected",
			Help:      "Count of the neighborhood being selected.",
		}),
		SampleDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "reserve_sample_duration",
			Help:      "Time taken to produce a reserve sample.",
		}),
		Round: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "round",
			Help:      "Current round calculated from the block height.",
		}),

		// total call
		BackendCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "backend_calls",
			Help:      "total chain backend calls",
		}),
		BackendErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "backend_errors",
			Help:      "total chain backend errors",
		}),

		// phase errors
		ErrReveal: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "reveal_phase_errors",
			Help:      "total reveal phase errors while processing",
		}),
		ErrCommit: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "commit_phase_errors",
			Help:      "total commit phase errors while processing",
		}),
		ErrClaim: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "claim_phase_errors",
			Help:      "total claim phase errors while processing",
		}),
		ErrWinner: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "win_phase_errors",
			Help:      "total win phase while processing",
		}),
		ErrCheckIsPlaying: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "is_playing_errors",
			Help:      "total neighborhood selected errors while processing",
		}),
	}
}

// TODO: register metric
func (a *Agent) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(a.metrics)
}
