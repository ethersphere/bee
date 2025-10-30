// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pullsync

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

type metrics struct {
	Offered              m.Counter             // number of chunks offered
	Wanted               m.Counter             // number of chunks wanted
	MissingChunks        m.Counter             // number of reserve get errs
	ReceivedZeroAddress  m.Counter             // number of delivered chunks with invalid address
	ReceivedInvalidChunk m.Counter             // number of delivered chunks with invalid address
	Delivered            m.Counter             // number of chunk deliveries
	SentOffered          m.Counter             // number of chunks offered
	SentWanted           m.Counter             // number of chunks wanted
	Sent                 m.Counter             // number of chunks sent
	DuplicateRuid        m.Counter             // number of duplicate RUID requests we got
	LastReceived         m.CounterMetricVector // last timestamp of the received chunks per bin
}

func newMetrics() metrics {
	subsystem := "pullsync"

	return metrics{
		Offered: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "chunks_offered",
			Help:      "Total chunks offered.",
		}),
		Wanted: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "chunks_wanted",
			Help:      "Total chunks wanted.",
		}),
		MissingChunks: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "missing_chunks",
			Help:      "Total reserve get errors.",
		}),
		ReceivedZeroAddress: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "received_zero_address",
			Help:      "Total chunks delivered with zero address and no chunk data.",
		}),
		ReceivedInvalidChunk: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "received_invalid_chunks",
			Help:      "Total invalid chunks delivered.",
		}),
		Delivered: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "chunks_delivered",
			Help:      "Total chunks delivered.",
		}),
		SentOffered: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "chunks_sent_offered",
			Help:      "Total chunks offered to peers.",
		}),
		SentWanted: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "chunks_sent_wanted",
			Help:      "Total chunks wanted by peers.",
		}),
		Sent: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "chunks_sent",
			Help:      "Total chunks sent.",
		}),
		DuplicateRuid: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "duplicate_ruids",
			Help:      "Total duplicate RUIDs.",
		}),
		LastReceived: m.NewCounterVec(
			m.CounterOpts{
				Namespace: m.Namespace,
				Subsystem: subsystem,
				Name:      "last_received",
				Help:      `The last timestamp of the received chunks per bin.`,
			}, []string{"bin"}),
	}
}

func (s *Syncer) Metrics() []m.Collector {
	return m.PrometheusCollectorsFromFields(s.metrics)
}
