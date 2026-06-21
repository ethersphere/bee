// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chainsim

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"math/rand"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

const SnapshotVersion = 2

// Snapshot is a JSON-serializable chain state.
type Snapshot struct {
	Version int `json:"version"`

	BlockNum uint64 `json:"block_num"`
	BlockTs  uint64 `json:"block_ts"`
	BaseFee  string `json:"base_fee"`

	Nonces       map[string]uint64            `json:"nonces"`
	NonceHistory map[string][]nonceRecordJSON `json:"nonce_history"`
	Balances     map[string]string            `json:"balances"`

	Blocks   []blockSnapshot        `json:"blocks"`
	Receipts []receiptSnapshot      `json:"receipts"`
	MinedTxs []minedTxSnapshot      `json:"mined_txs"`
	Mempool  []mempoolEntrySnapshot `json:"mempool"`

	Congestion          float64 `json:"congestion"`
	BackgroundTipMean   string  `json:"background_tip_mean"`
	BackgroundTipStdDev string  `json:"background_tip_stddev"`
	MinMempoolTip       string  `json:"min_mempool_tip"`
	EstimateGas         uint64  `json:"estimate_gas"`
	RNGSeed             int64   `json:"rng_seed"`

	RevertAddresses []string `json:"revert_addresses,omitempty"`

	Stats Stats `json:"stats"`
}

type nonceRecordJSON struct {
	BlockNum uint64 `json:"block_num"`
	Nonce    uint64 `json:"nonce"`
}

type blockSnapshot struct {
	Number   uint64   `json:"number"`
	Time     uint64   `json:"time"`
	BaseFee  string   `json:"base_fee"`
	GasUsed  uint64   `json:"gas_used"`
	GasLimit uint64   `json:"gas_limit"`
	Tips     []string `json:"tips"`
	TxHashes []string `json:"tx_hashes"`
}

type receiptSnapshot struct {
	TxHash      string `json:"tx_hash"`
	Status      uint64 `json:"status"`
	GasUsed     uint64 `json:"gas_used"`
	BlockNumber uint64 `json:"block_number"`
	IncludedAt  uint64 `json:"included_at"`
}

type minedTxSnapshot struct {
	Hash  string `json:"hash"`
	TxRLP string `json:"tx_rlp"`
}

type mempoolEntrySnapshot struct {
	TxRLP   string `json:"tx_rlp"`
	Sender  string `json:"sender"`
	AddedAt uint64 `json:"added_at"`
}

// SetBlockCommitHook registers a callback invoked after each committed block.
func (s *SimChain) SetBlockCommitHook(fn func(blockNum uint64)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onBlockCommit = fn
}

// Snapshot returns a copy of the current chain state.
func (s *SimChain) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	snap := Snapshot{
		Version:             SnapshotVersion,
		BlockNum:            s.blockNum,
		BlockTs:             s.blockTs,
		BaseFee:             s.baseFee.String(),
		Nonces:              make(map[string]uint64, len(s.nonces)),
		NonceHistory:        make(map[string][]nonceRecordJSON, len(s.nonceHistory)),
		Balances:            make(map[string]string, len(s.balances)),
		Congestion:          s.congestion,
		BackgroundTipMean:   s.backgroundTipMean.String(),
		BackgroundTipStdDev: s.backgroundTipStdDev.String(),
		MinMempoolTip:       s.minMempoolTip.String(),
		EstimateGas:         s.estimateGas,
		RNGSeed:             s.cfg.RNGSeed,
	}

	for addr, nonce := range s.nonces {
		snap.Nonces[addr.Hex()] = nonce
	}
	for addr, history := range s.nonceHistory {
		records := make([]nonceRecordJSON, len(history))
		for i, rec := range history {
			records[i] = nonceRecordJSON{BlockNum: rec.blockNum, Nonce: rec.nonce}
		}
		snap.NonceHistory[addr.Hex()] = records
	}
	for addr, bal := range s.balances {
		snap.Balances[addr.Hex()] = bal.String()
	}

	snap.Blocks = make([]blockSnapshot, len(s.blocks))
	for i, block := range s.blocks {
		bs := blockSnapshot{
			Number:   block.number,
			Time:     block.time,
			BaseFee:  block.baseFee.String(),
			GasUsed:  block.gasUsed,
			GasLimit: block.gasLimit,
			Tips:     bigIntsToStrings(block.tips),
			TxHashes: hashesToStrings(block.txHashes),
		}
		snap.Blocks[i] = bs
	}

	snap.Receipts = make([]receiptSnapshot, 0, len(s.receipts))
	for hash, rec := range s.receipts {
		snap.Receipts = append(snap.Receipts, receiptSnapshot{
			TxHash:      hash.Hex(),
			Status:      rec.receipt.Status,
			GasUsed:     rec.receipt.GasUsed,
			BlockNumber: rec.receipt.BlockNumber.Uint64(),
			IncludedAt:  rec.includedAt,
		})
	}

	snap.MinedTxs = make([]minedTxSnapshot, 0, len(s.minedTxs))
	for hash, tx := range s.minedTxs {
		snap.MinedTxs = append(snap.MinedTxs, minedTxSnapshot{
			Hash:  hash.Hex(),
			TxRLP: encodeTxRLP(tx),
		})
	}

	for _, entry := range s.pool.entries {
		snap.Mempool = append(snap.Mempool, mempoolEntrySnapshot{
			TxRLP:   encodeTxRLP(entry.tx),
			Sender:  entry.sender.Hex(),
			AddedAt: entry.addedAt,
		})
	}

	for addr := range s.revertAddresses {
		snap.RevertAddresses = append(snap.RevertAddresses, addr.Hex())
	}

	snap.Stats = s.stats.copy()

	return snap
}

// Restore creates a SimChain from configuration and a persisted snapshot.
func Restore(cfg Config, snap Snapshot) (*SimChain, error) {
	if snap.Version < 1 || snap.Version > SnapshotVersion {
		return nil, fmt.Errorf("unsupported snapshot version %d", snap.Version)
	}

	baseFee, ok := new(big.Int).SetString(snap.BaseFee, 10)
	if !ok {
		return nil, fmt.Errorf("invalid base fee %q", snap.BaseFee)
	}

	cfg = cfg.normalized()
	if snap.RNGSeed != 0 {
		cfg.RNGSeed = snap.RNGSeed
	}

	s := New(cfg)
	s.mu.Lock()
	defer s.mu.Unlock()

	s.blockNum = snap.BlockNum
	s.blockTs = snap.BlockTs
	s.baseFee = baseFee

	s.nonces = make(map[common.Address]uint64, len(snap.Nonces))
	for addrHex, nonce := range snap.Nonces {
		s.nonces[common.HexToAddress(addrHex)] = nonce
	}

	s.nonceHistory = make(map[common.Address][]nonceRecord, len(snap.NonceHistory))
	for addrHex, history := range snap.NonceHistory {
		addr := common.HexToAddress(addrHex)
		records := make([]nonceRecord, len(history))
		for i, rec := range history {
			records[i] = nonceRecord{blockNum: rec.BlockNum, nonce: rec.Nonce}
		}
		s.nonceHistory[addr] = records
	}

	s.balances = make(map[common.Address]*big.Int, len(snap.Balances))
	for addrHex, balStr := range snap.Balances {
		bal, ok := new(big.Int).SetString(balStr, 10)
		if !ok {
			return nil, fmt.Errorf("invalid balance for %s: %q", addrHex, balStr)
		}
		s.balances[common.HexToAddress(addrHex)] = bal
	}

	s.blocks = make([]*simBlock, 0, len(snap.Blocks))
	for _, bs := range snap.Blocks {
		blockBaseFee, ok := new(big.Int).SetString(bs.BaseFee, 10)
		if !ok {
			return nil, fmt.Errorf("invalid block base fee %q", bs.BaseFee)
		}
		s.blocks = append(s.blocks, &simBlock{
			number:   bs.Number,
			time:     bs.Time,
			baseFee:  blockBaseFee,
			gasUsed:  bs.GasUsed,
			gasLimit: bs.GasLimit,
			tips:     stringsToBigInts(bs.Tips),
			txHashes: stringsToHashes(bs.TxHashes),
		})
	}

	s.receipts = make(map[common.Hash]*receiptRecord, len(snap.Receipts))
	for _, rs := range snap.Receipts {
		hash := common.HexToHash(rs.TxHash)
		s.receipts[hash] = &receiptRecord{
			receipt: &types.Receipt{
				TxHash:      hash,
				Status:      rs.Status,
				GasUsed:     rs.GasUsed,
				BlockNumber: new(big.Int).SetUint64(rs.BlockNumber),
				BlockHash:   syntheticBlockHash(rs.BlockNumber),
			},
			includedAt: rs.IncludedAt,
		}
	}

	s.minedTxs = make(map[common.Hash]*types.Transaction, len(snap.MinedTxs))
	for _, mt := range snap.MinedTxs {
		tx, err := decodeTxRLP(mt.TxRLP)
		if err != nil {
			return nil, fmt.Errorf("decode mined tx %s: %w", mt.Hash, err)
		}
		s.minedTxs[common.HexToHash(mt.Hash)] = tx
	}

	s.pool = newMempool(cfg.MaxMempoolSize, cfg.MempoolTTL)
	for _, me := range snap.Mempool {
		tx, err := decodeTxRLP(me.TxRLP)
		if err != nil {
			return nil, fmt.Errorf("decode mempool tx: %w", err)
		}
		s.pool.insert(&poolEntry{
			tx:      tx,
			sender:  common.HexToAddress(me.Sender),
			addedAt: me.AddedAt,
		})
	}

	if tip, ok := new(big.Int).SetString(snap.MinMempoolTip, 10); ok {
		s.minMempoolTip = tip
	}
	if mean, ok := new(big.Int).SetString(snap.BackgroundTipMean, 10); ok {
		s.backgroundTipMean = mean
	}
	if stdDev, ok := new(big.Int).SetString(snap.BackgroundTipStdDev, 10); ok {
		s.backgroundTipStdDev = stdDev
	}
	s.congestion = snap.Congestion
	if snap.EstimateGas != 0 {
		s.estimateGas = snap.EstimateGas
	}

	s.revertAddresses = make(map[common.Address]struct{}, len(snap.RevertAddresses))
	for _, addrHex := range snap.RevertAddresses {
		s.revertAddresses[common.HexToAddress(addrHex)] = struct{}{}
	}

	s.rng = rand.New(rand.NewSource(cfg.RNGSeed)) //nolint:gosec // deterministic simulation RNG

	if snap.Version >= 2 {
		s.stats = snap.Stats.copy()
	}

	s.rebuildMinedOrderLocked()

	return s, nil
}

func encodeTxRLP(tx *types.Transaction) string {
	data, err := tx.MarshalBinary()
	if err != nil {
		return ""
	}
	return hex.EncodeToString(data)
}

func decodeTxRLP(encoded string) (*types.Transaction, error) {
	data, err := hex.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("hex decode: %w", err)
	}
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(data); err != nil {
		return nil, fmt.Errorf("unmarshal tx: %w", err)
	}
	return tx, nil
}

func bigIntsToStrings(vals []*big.Int) []string {
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i] = v.String()
	}
	return out
}

func stringsToBigInts(vals []string) []*big.Int {
	out := make([]*big.Int, len(vals))
	for i, v := range vals {
		bi, ok := new(big.Int).SetString(v, 10)
		if !ok {
			bi = new(big.Int)
		}
		out[i] = bi
	}
	return out
}

func hashesToStrings(hashes []common.Hash) []string {
	out := make([]string, len(hashes))
	for i, h := range hashes {
		out[i] = h.Hex()
	}
	return out
}

func stringsToHashes(vals []string) []common.Hash {
	out := make([]common.Hash, len(vals))
	for i, v := range vals {
		out[i] = common.HexToHash(v)
	}
	return out
}
