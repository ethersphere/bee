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

	// group 1: block binding
	PhaseDetectedBlock *prometheus.GaugeVec
	ActionBlockOffset  *prometheus.GaugeVec
	TxMinedBlockOffset *prometheus.GaugeVec

	// group 2: error classification
	WrongPhaseErrors *prometheus.CounterVec
	TxRevertedErrors *prometheus.CounterVec
	TxSendErrors     *prometheus.CounterVec

	// group 3: latency / timing
	HandlerLatencySeconds   *prometheus.HistogramVec
	HandlerDelaySeconds     *prometheus.HistogramVec
	SampleDurationSeconds   prometheus.Histogram
	TxConfirmationSeconds   *prometheus.HistogramVec
	IsPlayingLatencySeconds prometheus.Histogram

	// group 4: phase skips
	PhaseSkipped *prometheus.CounterVec

	// group 5: sample overlap
	SampleOverlap       prometheus.Counter
	SampleStartedRound  prometheus.Gauge
	SampleFinishedRound prometheus.Gauge
}

func newMetrics() metrics {
	subsystem := "storageincentives"

	durationBuckets := []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600}

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

		PhaseDetectedBlock: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "phase_detected_block",
			Help:      "Block number when a phase transition was detected.",
		}, []string{"phase"}),
		ActionBlockOffset: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "action_block_offset",
			Help:      "Block offset within the round (block % blocksPerRound) when a contract action was initiated.",
		}, []string{"action"}),
		TxMinedBlockOffset: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "tx_mined_block_offset",
			Help:      "Block offset within the round when a transaction was mined.",
		}, []string{"action"}),

		WrongPhaseErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "wrong_phase_errors_total",
			Help:      "Contract calls that failed due to wrong on-chain phase.",
		}, []string{"action"}),
		TxRevertedErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "tx_reverted_errors_total",
			Help:      "Contract transactions that reverted for reasons other than wrong phase.",
		}, []string{"action"}),
		TxSendErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "tx_send_errors_total",
			Help:      "Contract transaction send or confirmation failures that are not on-chain reverts.",
		}, []string{"action"}),

		HandlerLatencySeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "handler_latency_seconds",
			Help:      "Duration of phase handler execution.",
			Buckets:   durationBuckets,
		}, []string{"phase"}),
		HandlerDelaySeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "handler_delay_seconds",
			Help:      "Delay between phase detection and handler start.",
			Buckets:   durationBuckets,
		}, []string{"phase"}),
		SampleDurationSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "sample_duration_seconds",
			Help:      "Duration of reserve sample production.",
			Buckets:   durationBuckets,
		}),
		TxConfirmationSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "tx_confirmation_seconds",
			Help:      "Duration from transaction send to receipt confirmation.",
			Buckets:   durationBuckets,
		}, []string{"action"}),
		IsPlayingLatencySeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "is_playing_latency_seconds",
			Help:      "Duration of the IsPlaying eth_call.",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
		}),

		PhaseSkipped: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "phase_skipped_total",
			Help:      "Phase handlers skipped without attempting a contract action.",
		}, []string{"phase", "reason"}),

		SampleOverlap: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "sample_overlap_total",
			Help:      "Times sample production overlapped with the commit phase of the next round.",
		}),
		SampleStartedRound: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "sample_started_round",
			Help:      "Round number when the last sample handler started.",
		}),
		SampleFinishedRound: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "sample_finished_round",
			Help:      "Round number when the last sample handler finished.",
		}),
	}
}

func (a *Agent) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(a.metrics)
}
