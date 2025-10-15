// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package listener

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

type metrics struct {
	// aggregate events handled
	EventsProcessed m.Counter
	EventErrors     m.Counter
	PagesProcessed  m.Counter

	// individual event counters
	CreatedCounter m.Counter
	TopupCounter   m.Counter
	DepthCounter   m.Counter
	PriceCounter   m.Counter

	// total calls to chain backend
	BackendCalls  m.Counter
	BackendErrors m.Counter

	// processing durations
	PageProcessDuration  m.Counter
	EventProcessDuration m.Counter
}

func newMetrics() metrics {
	subsystem := "postage_listener"

	return metrics{
		// aggregate events handled
		EventsProcessed: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "events_processed",
			Help:      "total events processed",
		}),
		EventErrors: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "event_errors",
			Help:      "total event errors while processing",
		}),
		PagesProcessed: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "pages_processed",
			Help:      "total pages processed",
		}),

		// individual event counters
		CreatedCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "created_events",
			Help:      "total batch created events processed",
		}),

		TopupCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "topup_events",
			Help:      "total batch topup events handled",
		}),

		DepthCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "depth_events",
			Help:      "total batch depth change events handled",
		}),

		PriceCounter: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "price_events",
			Help:      "total price change events handled",
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

		// processing durations
		PageProcessDuration: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "page_duration",
			Help:      "how long it took to process a page",
		}),

		EventProcessDuration: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "event_duration",
			Help:      "how long it took to process a single event",
		}),
	}
}

func (l *listener) Metrics() []m.Collector {
	return m.PrometheusCollectorsFromFields(l.metrics)
}
