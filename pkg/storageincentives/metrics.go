// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	// aggregate events handled
	AllPhaseEvents        prometheus.Counter
	AllContractCalls      prometheus.Counter
	AllContractCallErrors prometheus.Counter

	// phase gauge and counter
	CurrentPhase         prometheus.Gauge
	RevealPhase          prometheus.Counter
	CommitPhase          prometheus.Counter
	ClaimPhase           prometheus.Counter
	Winner               prometheus.Counter
	NeighborhoodSelected prometheus.Counter
	SampleDuration       prometheus.Gauge
	Round                prometheus.Gauge

	// total calls to chain backend
	BackendCalls  prometheus.Counter
	BackendErrors prometheus.Counter

	// metrics for err processing
	PhasesErrors phasesErrors
}

type phasesErrors struct {
	Reveal               prometheus.Counter
	Commit               prometheus.Counter
	Claim                prometheus.Counter
	Winner               prometheus.Counter
	NeighborhoodSelected prometheus.Counter
	All                  prometheus.Counter
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

		// aggregate events handled
		AllPhaseEvents: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "events_processed",
			Help:      "total events processed",
		}),
		AllContractCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "contractCalls_processed",
			Help:      "total contract calls processed",
		}),
		AllContractCallErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "contractCallErrors_processed",
			Help:      "total contract call errors processed",
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
		PhasesErrors: phasesErrors{
			Reveal: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "revealPhase_errors",
				Help:      "total reveal phase errors while processing",
			}),
			Commit: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "commitPhase_errors",
				Help:      "total commit phase errors while processing",
			}),
			Claim: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "claimPhase_errors",
				Help:      "total claim phase errors while processing",
			}),
			Winner: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "winPhase_errors",
				Help:      "total win phase while processing",
			}),
			NeighborhoodSelected: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "neighborhoodSelected_errors",
				Help:      "total neighborhood selected errors while processing",
			}),
			All: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "event_errors",
				Help:      "total event errors while processing",
			}),
		},
	}
}

// TODO: register metric
func (s *Agent) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
