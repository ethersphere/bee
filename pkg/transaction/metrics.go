// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"context"
	"errors"

	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

var _ m.Collector = (*transactionService)(nil)

// transactionsWithRetryMetrics collects SendWithRetry monitoring data for Prometheus dashboards.
type transactionsWithRetryMetrics struct {
	// AttemptsPerTransaction is the number of broadcast rounds per SendWithRetry invocation
	// (1 = confirmed on the first broadcast, 2 = one retry, etc.).
	AttemptsPerTransaction prometheus.Histogram
	// OutcomesTotal counts finished SendWithRetry runs by result label.
	OutcomesTotal *prometheus.CounterVec
}

func newRetryMetrics() transactionsWithRetryMetrics {
	subsystem := "transaction_retry"
	return transactionsWithRetryMetrics{
		AttemptsPerTransaction: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "attempts_per_transaction",
			Help:      "Broadcast attempts per SendWithRetry invocation (1 = no retry needed).",
			Buckets:   []float64{1, 2, 3, 4, 5, 6, 8, 10},
		}),
		OutcomesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "outcomes_total",
			Help:      "Finished SendWithRetry invocations by outcome.",
		}, []string{"result"}),
	}
}

func (t *transactionService) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(t.metrics)
}

func (t *transactionService) recordRetryComplete(broadcastAttempts int, err error) {
	t.metrics.OutcomesTotal.WithLabelValues(retryOutcomeLabel(err)).Inc()
	if broadcastAttempts > 0 {
		t.metrics.AttemptsPerTransaction.Observe(float64(broadcastAttempts))
	}
}

// retryOutcomeLabel maps a SendWithRetry terminal error to a stable Prometheus label.
func retryOutcomeLabel(err error) string {
	if err == nil {
		return "success"
	}
	if errors.Is(err, ErrTransactionReverted) {
		return "reverted"
	}
	if errors.Is(err, ErrSignTransaction) {
		return "sign_failed"
	}
	if errors.Is(err, ErrTransactionCancelled) {
		return "cancelled"
	}
	if errors.Is(err, ErrTxMaxPriceExceeded) {
		return "max_price_exceeded"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "context_canceled"
	}
	if errors.Is(err, ErrAllAttemptsExhausted) {
		return "attempts_exhausted"
	}
	if containsNormalized(err.Error(), "sendtxswithretryrequiresautomaticgaspricing") {
		return "manual_gas_price"
	}
	if isNonRetryable(err) {
		return "critical"
	}
	return "other"
}
