// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package wrapped

import (
	m "github.com/ethersphere/bee/v2/pkg/metrics"
)

type metrics struct {
	TotalRPCCalls  m.Counter
	TotalRPCErrors m.Counter

	TransactionReceiptCalls m.Counter
	TransactionCalls        m.Counter
	BlockNumberCalls        m.Counter
	BlockHeaderCalls        m.Counter
	BalanceCalls            m.Counter
	NonceAtCalls            m.Counter
	PendingNonceCalls       m.Counter
	CallContractCalls       m.Counter
	SuggestGasTipCapCalls   m.Counter
	EstimateGasCalls        m.Counter
	SendTransactionCalls    m.Counter
	FilterLogsCalls         m.Counter
	ChainIDCalls            m.Counter
}

func newMetrics() metrics {
	subsystem := "eth_backend"

	return metrics{
		TotalRPCCalls: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_rpc_calls",
			Help:      "Count of rpc calls",
		}),
		TotalRPCErrors: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "total_rpc_errors",
			Help:      "Count of rpc errors",
		}),
		TransactionCalls: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_transaction",
			Help:      "Count of eth_getTransaction rpc calls",
		}),
		TransactionReceiptCalls: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_transaction_receipt",
			Help:      "Count of eth_getTransactionReceipt rpc errors",
		}),
		BlockNumberCalls: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_block_number",
			Help:      "Count of eth_blockNumber rpc calls",
		}),
		BlockHeaderCalls: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_block_header",
			Help:      "Count of eth_getBlockByNumber (header only) calls",
		}),
		BalanceCalls: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_balance",
			Help:      "Count of eth_getBalance rpc calls",
		}),
		NonceAtCalls: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_nonce_at",
			Help:      "Count of eth_getTransactionCount (pending false) rpc calls",
		}),
		PendingNonceCalls: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_pending_nonce_at",
			Help:      "Count of eth_getTransactionCount (pending true) rpc calls",
		}),
		CallContractCalls: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_eth_call",
			Help:      "Count of eth_call rpc calls",
		}),
		SuggestGasTipCapCalls: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_suggest_gas_tip_cap",
			Help:      "Count of eth_maxPriorityFeePerGas rpc calls",
		}),
		EstimateGasCalls: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_estimate_gasprice",
			Help:      "Count of eth_estimateGas rpc calls",
		}),
		SendTransactionCalls: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_send_transaction",
			Help:      "Count of eth_sendRawTransaction rpc calls",
		}),
		FilterLogsCalls: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_filter_logs",
			Help:      "Count of eth_getLogs rpc calls",
		}),
		ChainIDCalls: m.NewCounter(m.CounterOpts{
			Namespace: m.Namespace,
			Subsystem: subsystem,
			Name:      "calls_chain_id",
			Help:      "Count of eth_chainId rpc calls",
		}),
	}
}

func (b *wrappedBackend) Metrics() []m.Collector {
	return m.PrometheusCollectorsFromFields(b.metrics)
}
