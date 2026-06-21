// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/chainsim"
	"gopkg.in/yaml.v2"
)

const defaultRPCListen = "127.0.0.1:8545"

// YAMLConfig is the chainsim YAML configuration file format.
type YAMLConfig struct {
	ChainID       int64         `yaml:"chain_id"`
	BlockPeriod   time.Duration `yaml:"block_period"`
	BlockGasLimit uint64        `yaml:"block_gas_limit"`

	InitialBaseFee      string  `yaml:"initial_base_fee"`
	MinMempoolTip       string  `yaml:"min_mempool_tip"`
	Congestion          float64 `yaml:"congestion"`
	CongestionStdDev    float64 `yaml:"congestion_stddev"`
	BackgroundTipMean   string  `yaml:"background_tip_mean"`
	BackgroundTipStdDev string  `yaml:"background_tip_stddev"`

	MaxMempoolSize    int    `yaml:"max_mempool_size"`
	MempoolTTL        uint64 `yaml:"mempool_ttl"`
	ReceiptAvailDelay uint64 `yaml:"receipt_avail_delay"`
	EstimateGas       uint64 `yaml:"estimate_gas"`
	FeeHistoryDepth   int    `yaml:"fee_history_depth"`
	RNGSeed           int64  `yaml:"rng_seed"`

	InclusionProbability    bool          `yaml:"inclusion_probability"`
	InclusionMinProbability float64       `yaml:"inclusion_min_probability"`
	BlockPeriodJitter       time.Duration `yaml:"block_period_jitter"`
	MaxTxsPerBlock          int           `yaml:"max_txs_per_block"`

	RandomRevertRate float64 `yaml:"random_revert_rate"`

	RPC struct {
		Endpoint string `yaml:"endpoint"`
	} `yaml:"rpc"`

	StateDir string `yaml:"state_dir"`

	Accounts []AccountConfig `yaml:"accounts"`
}

// AccountConfig describes a genesis account.
type AccountConfig struct {
	Address string `yaml:"address"`
	Balance string `yaml:"balance"`
	Nonce   uint64 `yaml:"nonce"`
}

func loadYAMLConfig(path string) (YAMLConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return YAMLConfig{}, fmt.Errorf("read config: %w", err)
	}

	var cfg YAMLConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return YAMLConfig{}, fmt.Errorf("parse config: %w", err)
	}
	return cfg.normalized(), nil
}

func (c YAMLConfig) normalized() YAMLConfig {
	if c.ChainID == 0 {
		c.ChainID = 1337
	}
	if c.BlockPeriod <= 0 {
		c.BlockPeriod = 5 * time.Second
	}
	if c.BlockGasLimit == 0 {
		c.BlockGasLimit = 30_000_000
	}
	if c.InitialBaseFee == "" {
		c.InitialBaseFee = "1000000000"
	}
	if c.MinMempoolTip == "" {
		c.MinMempoolTip = "100000000"
	}
	if c.BackgroundTipMean == "" {
		c.BackgroundTipMean = "2000000000"
	}
	if c.BackgroundTipStdDev == "" {
		c.BackgroundTipStdDev = "500000000"
	}
	if c.EstimateGas == 0 {
		c.EstimateGas = 50_000
	}
	if c.FeeHistoryDepth == 0 {
		c.FeeHistoryDepth = 100
	}
	if c.RNGSeed == 0 {
		c.RNGSeed = 1
	}
	if c.RPC.Endpoint == "" {
		c.RPC.Endpoint = defaultRPCListen
	}
	if c.StateDir == "" {
		c.StateDir = "chainsim-state"
	}
	return c
}

func (c YAMLConfig) toSimConfig() (chainsim.Config, error) {
	initialBaseFee, err := parseBigInt(c.InitialBaseFee, "initial_base_fee")
	if err != nil {
		return chainsim.Config{}, err
	}
	minMempoolTip, err := parseBigInt(c.MinMempoolTip, "min_mempool_tip")
	if err != nil {
		return chainsim.Config{}, err
	}
	backgroundTipMean, err := parseBigInt(c.BackgroundTipMean, "background_tip_mean")
	if err != nil {
		return chainsim.Config{}, err
	}
	backgroundTipStdDev, err := parseBigInt(c.BackgroundTipStdDev, "background_tip_stddev")
	if err != nil {
		return chainsim.Config{}, err
	}

	return chainsim.Config{
		ChainID:                 big.NewInt(c.ChainID),
		BlockPeriod:             c.BlockPeriod,
		BlockGasLimit:           c.BlockGasLimit,
		InitialBaseFee:          initialBaseFee,
		MinMempoolTip:           minMempoolTip,
		InitialCongestion:       c.Congestion,
		CongestionStdDev:        c.CongestionStdDev,
		BackgroundTipMean:       backgroundTipMean,
		BackgroundTipStdDev:     backgroundTipStdDev,
		MaxMempoolSize:          c.MaxMempoolSize,
		MempoolTTL:              c.MempoolTTL,
		ReceiptAvailDelay:       c.ReceiptAvailDelay,
		EstimateGas:             c.EstimateGas,
		FeeHistoryDepth:         c.FeeHistoryDepth,
		RNGSeed:                 c.RNGSeed,
		InclusionProbability:    c.InclusionProbability,
		InclusionMinProbability: c.InclusionMinProbability,
		BlockPeriodJitter:       c.BlockPeriodJitter,
		MaxTxsPerBlock:          c.MaxTxsPerBlock,
		RandomRevertRate:        c.RandomRevertRate,
	}, nil
}

func applyGenesisAccounts(sim *chainsim.SimChain, accounts []AccountConfig) error {
	for _, acc := range accounts {
		addr := common.HexToAddress(acc.Address)
		if acc.Balance != "" {
			bal, err := parseBigInt(acc.Balance, "balance")
			if err != nil {
				return fmt.Errorf("account %s: %w", acc.Address, err)
			}
			sim.SetBalance(addr, bal)
		}
		if acc.Nonce > 0 {
			sim.SetNonce(addr, acc.Nonce)
		}
	}
	return nil
}

func parseBigInt(value, field string) (*big.Int, error) {
	bi, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return nil, fmt.Errorf("invalid %s: %q", field, value)
	}
	return bi, nil
}
