// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package accounting

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

type metrics struct {
	// all metrics fields must be exported
	// to be able to return them by Metrics()
	// using reflection
	TotalDebitedAmount                       m.Counter
	TotalCreditedAmount                      m.Counter
	DebitEventsCount                         m.Counter
	CreditEventsCount                        m.Counter
	AccountingDisconnectsEnforceRefreshCount m.Counter
	AccountingRefreshAttemptCount            m.Counter
	AccountingNonFatalRefreshFailCount       m.Counter
	AccountingDisconnectsOverdrawCount       m.Counter
	AccountingDisconnectsGhostOverdrawCount  m.Counter
	AccountingDisconnectsReconnectCount      m.Counter
	AccountingBlocksCount                    m.Counter
	AccountingReserveCount                   m.Counter
	TotalOriginatedCreditedAmount            m.Counter
	OriginatedCreditEventsCount              m.Counter
	SettleErrorCount                         m.Counter
	PaymentAttemptCount                      m.Counter
	PaymentErrorCount                        m.Counter
	ErrTimeOutOfSyncAlleged                  m.Counter
	ErrTimeOutOfSyncRecent                   m.Counter
	ErrTimeOutOfSyncInterval                 m.Counter
	ErrRefreshmentBelowExpected              m.Counter
	ErrRefreshmentAboveExpected              m.Counter
}

func newMetrics() metrics {
	subsystem := "accounting"

	return metrics{
		TotalDebitedAmount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_debited_amount",
			Help:      "Amount of BZZ debited to peers (potential income of the node)",
		}),
		TotalCreditedAmount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_credited_amount",
			Help:      "Amount of BZZ credited to peers (potential cost of the node)",
		}),
		DebitEventsCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "debit_events_count",
			Help:      "Number of occurrences of BZZ debit events towards peers",
		}),
		CreditEventsCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "credit_events_count",
			Help:      "Number of occurrences of BZZ credit events towards peers",
		}),
		AccountingDisconnectsEnforceRefreshCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disconnects_enforce_refresh_count",
			Help:      "Number of occurrences of peers disconnected based on failed refreshment attempts",
		}),
		AccountingRefreshAttemptCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "refresh_attempt_count",
			Help:      "Number of attempts of refresh op",
		}),
		AccountingNonFatalRefreshFailCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "non_fatal_refresh_fail_count",
			Help:      "Number of occurrences of refreshments failing for peers because of peer timestamp ahead of ours",
		}),
		AccountingDisconnectsOverdrawCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disconnects_overdraw_count",
			Help:      "Number of occurrences of peers disconnected based on payment thresholds",
		}),
		AccountingDisconnectsGhostOverdrawCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disconnects_ghost_overdraw_count",
			Help:      "Number of occurrences of peers disconnected based on undebitable requests thresholds",
		}),
		AccountingDisconnectsReconnectCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "disconnects_reconnect_count",
			Help:      "Number of occurrences of peers disconnected based on early attempt to reconnect",
		}),

		AccountingBlocksCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "accounting_blocks_count",
			Help:      "Number of occurrences of temporarily skipping a peer to avoid crossing their disconnect thresholds",
		}),
		AccountingReserveCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "accounting_reserve_count",
			Help:      "Number of reserve calls",
		}),
		TotalOriginatedCreditedAmount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_originated_credited_amount",
			Help:      "Amount of BZZ credited to peers (potential cost of the node) for originated traffic",
		}),
		OriginatedCreditEventsCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "originated_credit_events_count",
			Help:      "Number of occurrences of BZZ credit events as originator towards peers",
		}),
		SettleErrorCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "settle_error_count",
			Help:      "Number of errors occurring in settle method",
		}),
		PaymentErrorCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "payment_error_count",
			Help:      "Number of errors occurring during payment op",
		}),
		PaymentAttemptCount: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "payment_attempt_count",
			Help:      "Number of attempts of payment op",
		}),
		ErrRefreshmentBelowExpected: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "refreshment_below_expected",
			Help:      "Number of times the peer received a refreshment that is below expected",
		}),
		ErrRefreshmentAboveExpected: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "refreshment_above_expected",
			Help:      "Number of times the peer received a refreshment that is above expected",
		}),
		ErrTimeOutOfSyncAlleged: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "time_out_of_sync_alleged",
			Help:      "Number of times the timestamps from peer were decreasing",
		}),
		ErrTimeOutOfSyncRecent: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "time_out_of_sync_recent",
			Help:      "Number of times the timestamps from peer differed from our measurement by more than 2 seconds",
		}),
		ErrTimeOutOfSyncInterval: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "time_out_of_sync_interval",
			Help:      "Number of times the time interval from peer differed from local interval by more than 3 seconds",
		}),
	}
}

// Metrics returns the prometheus Collector for the accounting service.
func (a *Accounting) Metrics() []m.Collector {
	return m.PrometheusCollectorsFromFields(a.metrics)
}
