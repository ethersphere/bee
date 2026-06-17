// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chainsim

import (
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// Stats holds cumulative simulation counters persisted across restarts.
type Stats struct {
	BlocksProduced        uint64            `json:"blocks_produced"`
	TransactionsReceived  uint64            `json:"transactions_received"`
	TransactionsAccepted  uint64            `json:"transactions_accepted"`
	TransactionsRejected  uint64            `json:"transactions_rejected"`
	TransactionsExecuted  uint64            `json:"transactions_executed"`
	TransactionsReverted  uint64            `json:"transactions_reverted"`
	MempoolReplacements   uint64            `json:"mempool_replacements"`
	MempoolTTLExpirations uint64            `json:"mempool_ttl_expirations"`
	InclusionDeferred     uint64            `json:"inclusion_deferred"`
	RejectionReasons      map[string]uint64 `json:"rejection_reasons"`
}

func newStats() Stats {
	return Stats{RejectionReasons: make(map[string]uint64)}
}

func (s *SimChain) recordTxReceived() {
	s.stats.TransactionsReceived++
}

func (s *SimChain) recordTxAccepted(replaced bool) {
	s.stats.TransactionsAccepted++
	if replaced {
		s.stats.MempoolReplacements++
	}
}

func (s *SimChain) recordTxRejected(err error) {
	s.stats.TransactionsRejected++
	reason := classifyRejection(err)
	s.stats.RejectionReasons[reason]++
}

func (s *SimChain) recordTxExecuted(reverted bool) {
	s.stats.TransactionsExecuted++
	if reverted {
		s.stats.TransactionsReverted++
	}
}

func (s *SimChain) recordBlockProduced() {
	s.stats.BlocksProduced++
}

func (s *SimChain) recordMempoolTTLExpirations(count int) {
	s.stats.MempoolTTLExpirations += uint64(count)
}

func classifyRejection(err error) string {
	if err == nil {
		return "unknown"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "nonce too low"):
		return "nonce_too_low"
	case strings.Contains(msg, "max fee per gas less than block base fee"):
		return "fee_below_base_fee"
	case strings.Contains(msg, "max priority fee per gas too low"):
		return "tip_too_low"
	case strings.Contains(msg, "insufficient funds"):
		return "insufficient_funds"
	case strings.Contains(msg, "replacement transaction underpriced"):
		return "replacement_underpriced"
	case strings.Contains(msg, "mempool is full"):
		return "mempool_full"
	case strings.Contains(msg, "invalid transaction signature"):
		return "invalid_signature"
	default:
		return "other"
	}
}

func txLogFields(tx *types.Transaction, sender common.Address) []any {
	fields := []any{
		"tx_hash", tx.Hash(),
		"sender", sender,
		"nonce", tx.Nonce(),
		"gas", tx.Gas(),
		"gas_tip_cap", tx.GasTipCap(),
		"gas_fee_cap", tx.GasFeeCap(),
		"value", tx.Value(),
	}
	if to := tx.To(); to != nil {
		fields = append(fields, "to", *to)
	}
	return fields
}

// Stats returns a copy of cumulative simulation statistics.
func (s *SimChain) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats.copy()
}

func (s Stats) copy() Stats {
	out := s
	if s.RejectionReasons != nil {
		out.RejectionReasons = make(map[string]uint64, len(s.RejectionReasons))
		for k, v := range s.RejectionReasons {
			out.RejectionReasons[k] = v
		}
	} else {
		out.RejectionReasons = make(map[string]uint64)
	}
	return out
}
