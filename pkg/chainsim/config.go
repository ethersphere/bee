// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chainsim

import (
	"math/big"
	"time"
)

const (
	defaultBlockGasLimit   = 30_000_000
	defaultFeeHistoryDepth = 100
	defaultEstimateGas     = 50_000
)

// Config configures a simulated chain.
type Config struct {
	ChainID *big.Int

	// Block production.
	BlockPeriod   time.Duration
	BlockGasLimit uint64

	// EIP-1559 base fee at genesis.
	InitialBaseFee *big.Int

	// Mempool acceptance floor for priority fee.
	MinMempoolTip *big.Int

	// Fraction of block gas reserved for synthetic background traffic (0.0–1.0).
	InitialCongestion float64

	// Synthetic tip distribution used for fee history background traffic.
	BackgroundTipMean   *big.Int
	BackgroundTipStdDev *big.Int

	// Mempool limits. Zero disables the limit.
	MaxMempoolSize int
	// Evict txs not included after this many blocks. Zero disables TTL eviction.
	MempoolTTL uint64

	// Blocks after inclusion before TransactionReceipt returns a result. Zero is instant.
	ReceiptAvailDelay uint64

	// Default gas returned by EstimateGas when no override is configured.
	EstimateGas uint64

	// Number of historical blocks kept for fee history queries.
	FeeHistoryDepth int

	// Seed for synthetic background tip sampling. Zero uses a fixed default seed.
	RNGSeed int64

	// InclusionProbability enables probabilistic block inclusion when tip is below the network reference.
	InclusionProbability bool
	// InclusionMinProbability is the floor inclusion chance for low-tip transactions (0.0–1.0).
	InclusionMinProbability float64

	// BlockPeriodJitter adds uniform random delay in [-jitter, +jitter] to each block period. Zero disables jitter.
	BlockPeriodJitter time.Duration

	// MaxTxsPerBlock limits user transactions included per block. Zero means no limit.
	MaxTxsPerBlock int
}

// DefaultConfig returns a usable configuration for tests and local simulation.
func DefaultConfig() Config {
	return Config{
		ChainID:             big.NewInt(1337),
		BlockPeriod:         5 * time.Second,
		BlockGasLimit:       defaultBlockGasLimit,
		InitialBaseFee:      big.NewInt(1_000_000_000),
		MinMempoolTip:       big.NewInt(100_000_000),
		InitialCongestion:   0.0,
		BackgroundTipMean:   big.NewInt(2_000_000_000),
		BackgroundTipStdDev: big.NewInt(500_000_000),
		MaxMempoolSize:      0,
		MempoolTTL:          0,
		ReceiptAvailDelay:   0,
		EstimateGas:         defaultEstimateGas,
		FeeHistoryDepth:     defaultFeeHistoryDepth,
		RNGSeed:             1,
	}
}

func (c Config) normalized() Config {
	if c.ChainID == nil {
		c.ChainID = big.NewInt(1337)
	}
	if c.BlockPeriod <= 0 {
		c.BlockPeriod = 5 * time.Second
	}
	if c.BlockGasLimit == 0 {
		c.BlockGasLimit = defaultBlockGasLimit
	}
	if c.InitialBaseFee == nil {
		c.InitialBaseFee = big.NewInt(1_000_000_000)
	}
	if c.MinMempoolTip == nil {
		c.MinMempoolTip = big.NewInt(100_000_000)
	}
	if c.BackgroundTipMean == nil {
		c.BackgroundTipMean = big.NewInt(2_000_000_000)
	}
	if c.BackgroundTipStdDev == nil {
		c.BackgroundTipStdDev = big.NewInt(500_000_000)
	}
	if c.EstimateGas == 0 {
		c.EstimateGas = defaultEstimateGas
	}
	if c.FeeHistoryDepth == 0 {
		c.FeeHistoryDepth = defaultFeeHistoryDepth
	}
	if c.RNGSeed == 0 {
		c.RNGSeed = 1
	}
	if c.InitialCongestion < 0 {
		c.InitialCongestion = 0
	}
	if c.InitialCongestion > 1 {
		c.InitialCongestion = 1
	}
	if c.InclusionProbability && c.InclusionMinProbability <= 0 {
		c.InclusionMinProbability = defaultInclusionMinProbability
	}
	if c.InclusionMinProbability < 0 {
		c.InclusionMinProbability = 0
	}
	if c.InclusionMinProbability > 1 {
		c.InclusionMinProbability = 1
	}
	return c
}
