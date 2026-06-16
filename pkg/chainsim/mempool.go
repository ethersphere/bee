// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chainsim

import (
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

const replacementBumpPercent = 10

// poolEntry is a single transaction stored in the mempool.
type poolEntry struct {
	tx      *types.Transaction
	sender  common.Address
	addedAt uint64
}

func (e *poolEntry) effectiveTip(baseFee *big.Int) *big.Int {
	maxTip := new(big.Int).Sub(e.tx.GasFeeCap(), baseFee)
	if maxTip.Sign() <= 0 {
		return new(big.Int)
	}
	if e.tx.GasTipCap().Cmp(maxTip) < 0 {
		return new(big.Int).Set(e.tx.GasTipCap())
	}
	return maxTip
}

type mempool struct {
	entries   map[common.Hash]*poolEntry
	bySender  map[common.Address]map[uint64]*poolEntry
	maxSize   int
	ttlBlocks uint64
}

func newMempool(maxSize int, ttlBlocks uint64) *mempool {
	return &mempool{
		entries:   make(map[common.Hash]*poolEntry),
		bySender:  make(map[common.Address]map[uint64]*poolEntry),
		maxSize:   maxSize,
		ttlBlocks: ttlBlocks,
	}
}

func (m *mempool) insert(entry *poolEntry) {
	m.entries[entry.tx.Hash()] = entry
	if m.bySender[entry.sender] == nil {
		m.bySender[entry.sender] = make(map[uint64]*poolEntry)
	}
	m.bySender[entry.sender][entry.tx.Nonce()] = entry
}

func (m *mempool) remove(txHash common.Hash) {
	entry, ok := m.entries[txHash]
	if !ok {
		return
	}
	delete(m.entries, txHash)
	senderMap, ok := m.bySender[entry.sender]
	if !ok {
		return
	}
	delete(senderMap, entry.tx.Nonce())
	if len(senderMap) == 0 {
		delete(m.bySender, entry.sender)
	}
}

func (m *mempool) getByHash(txHash common.Hash) *poolEntry {
	return m.entries[txHash]
}

func (m *mempool) getBySenderNonce(sender common.Address, nonce uint64) *poolEntry {
	senderMap, ok := m.bySender[sender]
	if !ok {
		return nil
	}
	return senderMap[nonce]
}

func bump(val *big.Int, percent int) *big.Int {
	return new(big.Int).Div(
		new(big.Int).Mul(new(big.Int).Set(val), big.NewInt(int64(100+percent))),
		big.NewInt(100),
	)
}

func isValidReplacement(old, new *types.Transaction) bool {
	if new.GasTipCap().Cmp(bump(old.GasTipCap(), replacementBumpPercent)) < 0 {
		return false
	}
	if new.GasFeeCap().Cmp(bump(old.GasFeeCap(), replacementBumpPercent)) < 0 {
		return false
	}
	return true
}

func (m *mempool) add(entry *poolEntry, baseFee, minTip *big.Int, confirmedNonce uint64, balance *big.Int) error {
	if entry.tx.Nonce() < confirmedNonce {
		return errors.New("nonce too low")
	}

	if entry.tx.GasFeeCap().Cmp(baseFee) < 0 {
		return fmt.Errorf(
			"max fee per gas less than block base fee: address %s, maxFeePerGas: %s baseFee: %s",
			entry.sender.Hex(), entry.tx.GasFeeCap(), baseFee,
		)
	}

	tip := entry.effectiveTip(baseFee)
	if minTip != nil && tip.Cmp(minTip) < 0 {
		return fmt.Errorf(
			"max priority fee per gas too low: address %s, maxPriorityFeePerGas: %s minimum: %s",
			entry.sender.Hex(), entry.tx.GasTipCap(), minTip,
		)
	}

	cost := new(big.Int).Mul(new(big.Int).SetUint64(entry.tx.Gas()), entry.tx.GasFeeCap())
	cost.Add(cost, entry.tx.Value())
	if balance != nil && balance.Cmp(cost) < 0 {
		return fmt.Errorf(
			"insufficient funds for gas * price + value: address %s have %s want %s",
			entry.sender.Hex(), balance, cost,
		)
	}

	existing := m.getBySenderNonce(entry.sender, entry.tx.Nonce())
	if existing != nil {
		if !isValidReplacement(existing.tx, entry.tx) {
			return errors.New("replacement transaction underpriced")
		}
		m.remove(existing.tx.Hash())
	}

	if m.maxSize > 0 && len(m.entries) >= m.maxSize {
		evicted := m.evictLowestTip(baseFee)
		if evicted == nil {
			return errors.New("mempool is full")
		}
		if evicted.effectiveTip(baseFee).Cmp(tip) >= 0 {
			m.insert(evicted)
			return fmt.Errorf(
				"max priority fee per gas too low: mempool full, minimum tip to enter: %s",
				evicted.effectiveTip(baseFee),
			)
		}
	}

	m.insert(entry)
	return nil
}

func (m *mempool) evictLowestTip(baseFee *big.Int) *poolEntry {
	var lowest *poolEntry
	var lowestTip *big.Int

	for _, entry := range m.entries {
		tip := entry.effectiveTip(baseFee)
		if lowest == nil || tip.Cmp(lowestTip) < 0 {
			lowest = entry
			lowestTip = tip
		}
	}

	if lowest != nil {
		m.remove(lowest.tx.Hash())
	}
	return lowest
}

func (m *mempool) evictExpired(currentBlock uint64) []common.Hash {
	if m.ttlBlocks == 0 {
		return nil
	}

	var evicted []common.Hash
	for hash, entry := range m.entries {
		if currentBlock-entry.addedAt > m.ttlBlocks {
			m.remove(hash)
			evicted = append(evicted, hash)
		}
	}
	return evicted
}

func (m *mempool) eligible(confirmedNonces map[common.Address]uint64, baseFee *big.Int) []*poolEntry {
	result := make([]*poolEntry, 0, len(m.entries))

	for sender, nonceMap := range m.bySender {
		nextNonce := confirmedNonces[sender]
		entry, ok := nonceMap[nextNonce]
		if !ok {
			continue
		}
		if entry.tx.GasFeeCap().Cmp(baseFee) < 0 {
			continue
		}
		result = append(result, entry)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].effectiveTip(baseFee).Cmp(result[j].effectiveTip(baseFee)) > 0
	})

	return result
}

func (m *mempool) pendingNonce(sender common.Address, confirmedNonce uint64) uint64 {
	senderMap, ok := m.bySender[sender]
	if !ok {
		return confirmedNonce
	}

	nonce := confirmedNonce
	for {
		if _, exists := senderMap[nonce]; !exists {
			return nonce
		}
		nonce++
	}
}

func (m *mempool) size() int {
	return len(m.entries)
}

func (m *mempool) txHashes() []common.Hash {
	hashes := make([]common.Hash, 0, len(m.entries))
	for hash := range m.entries {
		hashes = append(hashes, hash)
	}
	return hashes
}
