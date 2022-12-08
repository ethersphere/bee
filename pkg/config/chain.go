// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	_ "embed"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/go-storage-incentives-abi/abi"
)

// TODO: replace this with proper values once the contracts are deployed to the mainnet.
var (
	//go:embed mainnet_staking_abi.json
	MainnetStakingABI string

	//go:embed mainnet_postagestamp_abi.json
	MainnetPostageStampABI string

	//go:embed mainnet_redistribution_abi.json
	MainnetRedistributionABI string
)

// TODO: consider adding BzzAddress (also as a cmd param) to the ChainConfig and remove the postagecontract.LookupERC20Address function.

type ChainConfig struct {
	// General.
	ChainID                int64
	PostageStampStartBlock uint64

	// Addresses.
	StakingAddress         common.Address
	PostageStampAddress    common.Address
	RedistributionAddress  common.Address
	SwapPriceOracleAddress common.Address
	CurrentFactoryAddress  common.Address
	LegacyFactoryAddresses []common.Address

	// ABIs.
	StakingABI        string
	PostageStampABI   string
	RedistributionABI string
}

var (
	Testnet = ChainConfig{
		ChainID:                abi.TestnetChainID,
		PostageStampStartBlock: abi.TestnetPostageStampBlockNumber,

		StakingAddress:         common.HexToAddress(abi.TestnetStakingAddress),
		PostageStampAddress:    common.HexToAddress(abi.TestnetPostageStampStampAddress),
		RedistributionAddress:  common.HexToAddress(abi.TestnetRedistributionAddress),
		SwapPriceOracleAddress: common.HexToAddress("0x0c9de531dcb38b758fe8a2c163444a5e54ee0db2"),
		CurrentFactoryAddress:  common.HexToAddress("0x73c412512E1cA0be3b89b77aB3466dA6A1B9d273"),
		LegacyFactoryAddresses: []common.Address{
			common.HexToAddress("0xf0277caffea72734853b834afc9892461ea18474"),
		},

		StakingABI:        abi.TestnetStakingABI,
		PostageStampABI:   abi.TestnetPostageStampStampABI,
		RedistributionABI: abi.TestnetRedistributionABI,
	}

	Mainnet = ChainConfig{
		ChainID:                100,
		PostageStampStartBlock: uint64(24180961),

		StakingAddress:         common.HexToAddress("0x52e86336210bB8F1FDe11EB8bc664a20AfC0a614"),
		PostageStampAddress:    common.HexToAddress("0xa9c84e9ccC0A0bC9B8C8E948F24E024bC2607c9A"),
		RedistributionAddress:  common.HexToAddress("0xECD2CFfE749A0F8F0a4f136E98C49De0Ee527c1F"),
		SwapPriceOracleAddress: common.HexToAddress("0x0FDc5429C50e2a39066D8A94F3e2D2476fcc3b85"),
		CurrentFactoryAddress:  common.HexToAddress("0xc2d5a532cf69aa9a1378737d8ccdef884b6e7420"),

		StakingABI:        MainnetStakingABI,
		PostageStampABI:   MainnetPostageStampABI,
		RedistributionABI: MainnetRedistributionABI,
	}
)

func GetByChainID(chainID int64) (ChainConfig, bool) {
	switch chainID {
	case Testnet.ChainID:
		return Testnet, true
	case Mainnet.ChainID:
		return Mainnet, true
	default:
		return ChainConfig{
			StakingABI:        abi.TestnetStakingABI,
			PostageStampABI:   abi.TestnetPostageStampStampABI,
			RedistributionABI: abi.TestnetRedistributionABI,
		}, false
	}
}
