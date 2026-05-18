// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"context"
	"errors"
	"math/big"
	"strconv"
	"strings"

	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

var _ m.Collector = (*transactionService)(nil)

// retryMetrics collects SendWithRetry monitoring data for Prometheus dashboards.
type retryMetrics struct {
	// AttemptsPerTransaction is the number of broadcast rounds per SendWithRetry invocation
	// (1 = confirmed on the first broadcast, 2 = one retry, etc.).
	AttemptsPerTransaction prometheus.Histogram
	// OutcomesTotal counts finished SendWithRetry runs by result label.
	OutcomesTotal *prometheus.CounterVec
	// BroadcastGasTipCap records maxPriorityFeePerGas (wei) per broadcast attempt index.
	BroadcastGasTipCap *prometheus.HistogramVec
	// BroadcastGasFeeCap records maxFeePerGas (wei) per broadcast attempt index.
	BroadcastGasFeeCap *prometheus.HistogramVec
}

func newRetryMetrics() retryMetrics {
	subsystem := "transaction_retry"

	// Gas fees on Gnosis/mainnet-style chains: from ~1 gwei to tens of gwei per unit.
	gasBuckets := prometheus.ExponentialBuckets(1_000_000_000, 2, 14)

	return retryMetrics{
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
		BroadcastGasTipCap: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "broadcast_gas_tip_cap_wei",
			Help:      "maxPriorityFeePerGas (wei) of each retry broadcast, labelled by attempt index (0 = first).",
			Buckets:   gasBuckets,
		}, []string{"attempt"}),
		BroadcastGasFeeCap: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "broadcast_gas_fee_cap_wei",
			Help:      "maxFeePerGas (wei) of each retry broadcast, labelled by attempt index (0 = first).",
			Buckets:   gasBuckets,
		}, []string{"attempt"}),
	}
}

func (t *transactionService) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(t.metrics)
}

func (t *transactionService) recordRetryBroadcast(attempt int, tip, feeCap *big.Int) {
	if tip == nil || feeCap == nil {
		return
	}
	attemptLabel := strconv.Itoa(attempt)
	t.metrics.BroadcastGasTipCap.WithLabelValues(attemptLabel).Observe(weiToFloat(tip))
	t.metrics.BroadcastGasFeeCap.WithLabelValues(attemptLabel).Observe(weiToFloat(feeCap))
}

func (t *transactionService) recordRetryComplete(broadcastAttempts int, err error) {
	t.metrics.OutcomesTotal.WithLabelValues(retryOutcomeLabel(err)).Inc()
	if broadcastAttempts > 0 {
		t.metrics.AttemptsPerTransaction.Observe(float64(broadcastAttempts))
	}
}

func weiToFloat(v *big.Int) float64 {
	if v == nil {
		return 0
	}
	f, _ := new(big.Float).SetInt(v).Float64()
	return f
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
	if strings.Contains(err.Error(), "transaction failed after") {
		return "attempts_exhausted"
	}
	if strings.Contains(err.Error(), "send txs with retry requires automatic gas pricing") {
		return "manual_gas_price"
	}
	if isErrCritical(err) {
		return "critical"
	}
	return "other"
}
