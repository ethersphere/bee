// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wrapped

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
	"github.com/ethersphere/bee/v2/pkg/transaction"
	"github.com/prometheus/client_golang/prometheus"
)

var _ transaction.RetryMetricsRecorder = (*metrics)(nil)

type metrics struct {
	TotalRPCCalls  prometheus.Counter
	TotalRPCErrors prometheus.Counter

	TransactionReceiptCalls       prometheus.Counter
	TransactionCalls              prometheus.Counter
	BlockHeaderAsBlockNumberCalls prometheus.Counter
	BlockHeaderCalls              prometheus.Counter
	BalanceCalls                  prometheus.Counter
	NonceAtCalls                  prometheus.Counter
	PendingNonceCalls             prometheus.Counter
	CallContractCalls             prometheus.Counter
	CodeAtCalls                   prometheus.Counter
	SuggestGasTipCapCalls         prometheus.Counter
	EstimateGasCalls              prometheus.Counter
	SendTransactionCalls          prometheus.Counter
	FilterLogsCalls               prometheus.Counter
	ChainIDCalls                  prometheus.Counter
	FeeHistoryCalls               prometheus.Counter
	FeeHistoryParseErrors         prometheus.Counter

	SendWithRetryAttemptsPerTransaction prometheus.Histogram
	SendWithRetryOutcomesTotal          *prometheus.CounterVec
}

func newMetrics() metrics {
	subsystem := "eth_backend"
	retrySubsystem := "transaction_retry"

	return metrics{
		TotalRPCCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_rpc_calls",
			Help:      "Count of rpc calls",
		}),
		TotalRPCErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_rpc_errors",
			Help:      "Count of rpc errors",
		}),
		TransactionCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_transaction",
			Help:      "Count of eth_getTransaction rpc calls",
		}),
		TransactionReceiptCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_transaction_receipt",
			Help:      "Count of eth_getTransactionReceipt rpc errors",
		}),
		BlockHeaderAsBlockNumberCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_block_header_as_block_number",
			Help:      "Count of eth_getBlockByNumber for getting block number rpc calls",
		}),
		BlockHeaderCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_block_header",
			Help:      "Count of eth_getBlockByNumber (header only) calls",
		}),
		BalanceCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_balance",
			Help:      "Count of eth_getBalance rpc calls",
		}),
		NonceAtCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_nonce_at",
			Help:      "Count of eth_getTransactionCount (pending false) rpc calls",
		}),
		PendingNonceCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_pending_nonce_at",
			Help:      "Count of eth_getTransactionCount (pending true) rpc calls",
		}),
		CallContractCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_eth_call",
			Help:      "Count of eth_call rpc calls",
		}),
		CodeAtCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_code_at",
			Help:      "Count of eth_getCode rpc calls",
		}),
		SuggestGasTipCapCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_suggest_gas_tip_cap",
			Help:      "Count of eth_maxPriorityFeePerGas rpc calls",
		}),
		EstimateGasCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_estimate_gasprice",
			Help:      "Count of eth_estimateGas rpc calls",
		}),
		SendTransactionCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_send_transaction",
			Help:      "Count of eth_sendRawTransaction rpc calls",
		}),
		FilterLogsCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_filter_logs",
			Help:      "Count of eth_getLogs rpc calls",
		}),
		ChainIDCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_chain_id",
			Help:      "Count of eth_chainId rpc calls",
		}),
		FeeHistoryCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_fee_history",
			Help:      "Count of eth_feeHistory rpc calls",
		}),
		FeeHistoryParseErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "fee_history_parse_errors",
			Help:      "Count of failures to derive suggested fees from fee history response",
		}),
		SendWithRetryAttemptsPerTransaction: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: m.Namespace,
			Subsystem: retrySubsystem,
			Name:      "attempts_per_transaction",
			Help:      "Broadcast attempts per SendWithRetry invocation (1 = no retry needed).",
			Buckets:   []float64{1, 2, 3, 4, 5, 6, 8, 10},
		}),
		SendWithRetryOutcomesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: retrySubsystem,
			Name:      "outcomes_total",
			Help:      "Finished SendWithRetry invocations by outcome.",
		}, []string{"result"}),
	}
}

func (m *metrics) RecordRetryComplete(broadcastAttempts int, err error) {
	m.SendWithRetryOutcomesTotal.WithLabelValues(transaction.RetryOutcomeLabel(err)).Inc()
	if broadcastAttempts > 0 {
		m.SendWithRetryAttemptsPerTransaction.Observe(float64(broadcastAttempts))
	}
}

func (b *wrappedBackend) Metrics() []prometheus.Collector {
	collectors := m.PrometheusCollectorsFromFields(b.metrics)
	collectors = append(collectors, b.blockNumberCache.Collectors()...)
	return collectors
}

func (b *wrappedBackend) RetryMetrics() transaction.RetryMetricsRecorder {
	return &b.metrics
}
