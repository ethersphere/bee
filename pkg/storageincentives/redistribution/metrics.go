// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redistribution

import (
	"context"
	"errors"
	"strings"

	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/prometheus/client_golang/prometheus"
)

// Tx kinds for metric labels (commit / reveal / claim).
const (
	TxKindCommit = "commit"
	TxKindReveal = "reveal"
	TxKindClaim  = "claim"
)

var (
	prepareSendRetries = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: "redistribution",
			Name:      "prepare_send_retries_total",
			Help:      "Waits before Send while estimated tx cost is above max-tx-cost (per tx kind).",
		},
		[]string{"tx_kind"},
	)

	claimCostWaits = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: "redistribution",
			Name:      "claim_cost_wait_iterations_total",
			Help:      "Iterations where Claim waited because estimated tx cost exceeded max-tx-cost (before override).",
		},
	)

	claimMaxTxCostOverrides = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: "redistribution",
			Name:      "claim_max_tx_cost_overrides_total",
			Help:      "Times Claim bypassed max-tx-cost because expected reward covered estimated cost plus round fees.",
		},
	)

	phaseSkippedExpensive = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: "redistribution",
			Name:      "phase_skipped_expensive_total",
			Help:      "Phases stopped while fees exceeded user max-tx-cost (or claim context ended while waiting).",
		},
		[]string{"phase"},
	)

	sendRetryStages = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: "redistribution",
			Name:      "send_retry_stages_total",
			Help:      "Send retry paths: empty_tx_hash_wait, resend_retry, resend_ok.",
		},
		[]string{"tx_kind", "stage"},
	)

	txErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: "redistribution",
			Name:      "errors_total",
			Help:      "Errors observed during redistribution txs (each failed Send/Resend/WaitForReceipt counts once).",
		},
		[]string{"tx_kind", "error_class"},
	)
)

func incPrepareSendRetry(txKind string) {
	prepareSendRetries.WithLabelValues(txKind).Inc()
}

func incClaimCostWait() {
	claimCostWaits.Inc()
}

func incClaimMaxTxCostOverride() {
	claimMaxTxCostOverrides.Inc()
}

func incPhaseSkippedExpensive(phase string) {
	phaseSkippedExpensive.WithLabelValues(phase).Inc()
}

func incSendRetryStage(txKind, stage string) {
	sendRetryStages.WithLabelValues(txKind, stage).Inc()
}

func incTxError(txKind string, err error) {
	if err == nil {
		return
	}
	txErrors.WithLabelValues(txKind, classifyErr(err)).Inc()
}

// classifyErr maps errors to a small label set for dashboards (low cardinality).
func classifyErr(err error) string {
	if err == nil {
		return "none"
	}
	if errors.Is(err, context.Canceled) {
		return "context_canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "context_deadline"
	}
	if errors.Is(err, ErrMaxTxCostExceeded) {
		return "max_tx_cost_exceeded"
	}
	if errors.Is(err, ErrZeroGasPrice) {
		return "zero_gas_price"
	}
	if errors.Is(err, transaction.ErrFeeCapExceeded) {
		return "fee_cap_exceeded"
	}
	if errors.Is(err, transaction.ErrTransactionReverted) {
		return "tx_reverted"
	}
	if errors.Is(err, transaction.ErrTransactionCancelled) {
		return "tx_cancelled"
	}

	s := err.Error()
	switch {
	case strings.Contains(s, "below current base fee"):
		return "below_base_fee"
	case strings.Contains(s, "specified gas price"):
		return "specified_gas_price"
	case strings.Contains(s, "insufficient funds"):
		return "insufficient_funds"
	case strings.Contains(s, "execution reverted"):
		return "execution_reverted"
	default:
		return "other"
	}
}

// Metrics returns Prometheus collectors for this package (register once).
func Metrics() []prometheus.Collector {
	return []prometheus.Collector{
		prepareSendRetries,
		claimCostWaits,
		claimMaxTxCostOverrides,
		phaseSkippedExpensive,
		sendRetryStages,
		txErrors,
	}
}
