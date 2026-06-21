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
	defaultMempoolDuration = 10 * time.Minute

	// DisabledMempoolTTL disables mempool TTL eviction when set as Config.MempoolTTL.
	DisabledMempoolTTL = ^uint64(0)
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

	// CongestionStdDev adds per-block Gaussian noise to congestion so that
	// background gas fluctuates around the mean rather than sitting exactly on
	// the gas target. This lets base fee decrease when the mempool is empty,
	// matching real EIP-1559 behavior. Zero disables noise.
	CongestionStdDev float64

	// Synthetic tip distribution used for fee history background traffic.
	BackgroundTipMean   *big.Int
	BackgroundTipStdDev *big.Int

	// Mempool limits. Zero disables the limit.
	MaxMempoolSize int
	// Evict txs not included after this many blocks. Zero uses a default of 10 minutes
	// based on BlockPeriod. Set to DisabledMempoolTTL to disable TTL eviction.
	MempoolTTL uint64

	// Blocks after inclusion before TransactionReceipt returns a result. Zero is instant.
	ReceiptAvailDelay uint64

	// Default gas returned by EstimateGas when no override is configured.
	EstimateGas uint64

	// BaseGasUsed is the gas charged per executed transaction. Zero uses the transaction gas limit.
	BaseGasUsed uint64

	// Number of historical blocks kept for fee history queries.
	FeeHistoryDepth int

	// HistoryRetentionBlocks bounds how many recent blocks of receipts, mined txs,
	// and nonce history are retained in memory. Zero keeps full history.
	HistoryRetentionBlocks uint64

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

	// RandomRevertRate is the probability (0.0–1.0) that any executed transaction reverts. Zero disables random reverts.
	RandomRevertRate float64
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
		BaseGasUsed:         21_000,
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
	if c.MempoolTTL == 0 {
		blocksIn10Min := uint64(defaultMempoolDuration / c.BlockPeriod)
		if blocksIn10Min == 0 {
			blocksIn10Min = 1
		}
		c.MempoolTTL = blocksIn10Min
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
	if c.CongestionStdDev < 0 {
		c.CongestionStdDev = 0
	}
	if c.CongestionStdDev > 0.5 {
		c.CongestionStdDev = 0.5
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
	if c.RandomRevertRate < 0 {
		c.RandomRevertRate = 0
	}
	if c.RandomRevertRate > 1 {
		c.RandomRevertRate = 1
	}
	return c
}
