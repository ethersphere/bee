// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transaction

import (
	"context"
	"errors"
)

// RetryMetricsRecorder records SendWithRetry completion metrics.
type RetryMetricsRecorder interface {
	RecordRetryComplete(broadcastAttempts int, err error)
}

type noopRetryMetricsRecorder struct{}

func (noopRetryMetricsRecorder) RecordRetryComplete(int, error) {}

type retryMetricsBackend interface {
	RetryMetrics() RetryMetricsRecorder
}

func retryMetricsFromBackend(backend Backend) RetryMetricsRecorder {
	if p, ok := backend.(retryMetricsBackend); ok {
		return p.RetryMetrics()
	}
	return noopRetryMetricsRecorder{}
}

// RetryOutcomeLabel maps a SendWithRetry terminal error to a stable Prometheus label.
func RetryOutcomeLabel(err error) string {
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
