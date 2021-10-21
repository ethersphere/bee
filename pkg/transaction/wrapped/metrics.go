// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wrapped

import (
	m "github.com/ethersphere/bee/pkg/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	TotalRPCCalls  prometheus.Counter
	TotalRPCErrors prometheus.Counter

	TransactionReceiptCalls prometheus.Counter
	TransactionCalls        prometheus.Counter
	BlockNumberCalls        prometheus.Counter
	BlockHeaderCalls        prometheus.Counter
	BalanceCalls            prometheus.Counter
	CodeAtCalls             prometheus.Counter
	NonceAtCalls            prometheus.Counter
	PendingNonceCalls       prometheus.Counter
	CallContractCalls       prometheus.Counter
	SuggestGasPriceCalls    prometheus.Counter
	EstimateGasCalls        prometheus.Counter
	SendTransactionCalls    prometheus.Counter
	FilterLogsCalls         prometheus.Counter
	ChainIDCalls            prometheus.Counter
}

func newMetrics() metrics {
	subsystem := "wrapped"

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
		BlockNumberCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_block_number",
			Help:      "Count of eth_blockNumber rpc calls",
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
		CodeAtCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_code_at",
			Help:      "Count of eth_getCode rpc calls",
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
		SuggestGasPriceCalls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_suggest_gasprice",
			Help:      "Count of eth_suggestGasPrice rpc calls",
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
	}
}

func (b *wrappedBackend) Metrics() []prometheus.Collector {
	return m.PrometheusCollectorsFromFields(b.metrics)
}
