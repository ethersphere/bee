// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"github.com/ethereum/go-ethereum/common"
)

var (
	// chain ID
	goerliChainID = int64(5)
	xdaiChainID   = int64(100)
	// start block
	// replace this with
	goerliStartBlock = uint64(7865424)
	xdaiStartBlock   = uint64(24180961)
	// factory address
	goerliContractAddress      = common.HexToAddress("0x0c9de531dcb38b758fe8a2c163444a5e54ee0db2")
	xdaiContractAddress        = common.HexToAddress("0x0FDc5429C50e2a39066D8A94F3e2D2476fcc3b85")
	goerliFactoryAddress       = common.HexToAddress("0x73c412512E1cA0be3b89b77aB3466dA6A1B9d273")
	xdaiFactoryAddress         = common.HexToAddress("0xc2d5a532cf69aa9a1378737d8ccdef884b6e7420")
	goerliLegacyFactoryAddress = common.HexToAddress("0xf0277caffea72734853b834afc9892461ea18474")
	// postage stamp
	goerliPostageStampAddress = common.HexToAddress("0x7aac0f092f7b961145900839ed6d54b1980f200c")
	xdaiPostageStampAddress   = common.HexToAddress("0xa9c84e9ccC0A0bC9B8C8E948F24E024bC2607c9A")

	//staking contract
	goerliStakingAddress = common.HexToAddress("0x18391158435582D5bE5ac1640ab5E2825F68d3a4")
	xdaiStakingAddress   = common.HexToAddress("0x52e86336210bB8F1FDe11EB8bc664a20AfC0a614")

	// redistribution contract
	goerliRedistributionAddress = common.HexToAddress("0x2e4ccEb9a88bcfbC7779Fa693714783A345c1343")
	xdaiRedistributionAddress   = common.HexToAddress("0xECD2CFfE749A0F8F0a4f136E98C49De0Ee527c1F")
)

type ChainConfig struct {
	StartBlock      uint64
	LegacyFactories []common.Address
	PostageStamp    common.Address
	Staking         common.Address
	CurrentFactory  common.Address
	PriceOracle     common.Address
	Redistribution  common.Address
}

func GetChainConfig(chainID int64) (*ChainConfig, bool) {
	var cfg ChainConfig
	switch chainID {
	case goerliChainID:
		cfg.PostageStamp = goerliPostageStampAddress
		cfg.StartBlock = goerliStartBlock
		cfg.CurrentFactory = goerliFactoryAddress
		cfg.LegacyFactories = []common.Address{
			goerliLegacyFactoryAddress,
		}
		cfg.PriceOracle = goerliContractAddress
		cfg.Staking = goerliStakingAddress
		cfg.Redistribution = goerliRedistributionAddress
		return &cfg, true
	case xdaiChainID:
		cfg.PostageStamp = xdaiPostageStampAddress
		cfg.StartBlock = xdaiStartBlock
		cfg.CurrentFactory = xdaiFactoryAddress
		cfg.LegacyFactories = []common.Address{}
		cfg.PriceOracle = xdaiContractAddress
		cfg.Staking = xdaiStakingAddress
		cfg.Redistribution = xdaiRedistributionAddress
		return &cfg, true
	default:
		return &cfg, false
	}
}
