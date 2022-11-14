// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	"github.com/ethereum/go-ethereum/common"
)

type ChainConfig struct {
	ChainID                int64
	PostageStampStartBlock uint64
	LegacyFactories        []common.Address
	PostageStamp           common.Address
	Staking                common.Address
	CurrentFactory         common.Address
	SwapPriceOracle        common.Address
	Redistribution         common.Address
}

var (
	goerliCfg = ChainConfig{
		ChainID:         5,
		SwapPriceOracle: common.HexToAddress("0x0c9de531dcb38b758fe8a2c163444a5e54ee0db2"),
		CurrentFactory:  common.HexToAddress("0x73c412512E1cA0be3b89b77aB3466dA6A1B9d273"),
		LegacyFactories: []common.Address{
			common.HexToAddress("0xf0277caffea72734853b834afc9892461ea18474"),
		},
		PostageStamp:           common.HexToAddress("0x7aac0f092f7b961145900839ed6d54b1980f200c"),
		PostageStampStartBlock: uint64(7590423),
		Staking:                common.HexToAddress("0x18391158435582D5bE5ac1640ab5E2825F68d3a4"),
		Redistribution:         common.HexToAddress("0x7d5ff32e744340ab26873d05e019a0d27fe4716f"),
	}

	xdaiCfg = ChainConfig{
		ChainID:                100,
		SwapPriceOracle:        common.HexToAddress("0x0FDc5429C50e2a39066D8A94F3e2D2476fcc3b85"),
		CurrentFactory:         common.HexToAddress("0xc2d5a532cf69aa9a1378737d8ccdef884b6e7420"),
		LegacyFactories:        []common.Address{},
		PostageStamp:           common.HexToAddress("0xa9c84e9ccC0A0bC9B8C8E948F24E024bC2607c9A"),
		PostageStampStartBlock: uint64(24180961),
		Staking:                common.HexToAddress("0x52e86336210bB8F1FDe11EB8bc664a20AfC0a614"),
		Redistribution:         common.HexToAddress("0xECD2CFfE749A0F8F0a4f136E98C49De0Ee527c1F"),
	}
)

var (
	MainnetChainID = xdaiCfg.ChainID
	TestnetChainID = goerliCfg.ChainID
)

func GetChainConfig(chainID int64) (ChainConfig, bool) {
	switch chainID {
	case goerliCfg.ChainID:
		return goerliCfg, true
	case xdaiCfg.ChainID:
		return xdaiCfg, true
	default:
		return ChainConfig{}, false
	}
}
