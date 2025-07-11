// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	_ "embed"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/go-storage-incentives-abi/abi"
)

// TODO: consider adding BzzAddress (also as a cmd param) to the ChainConfig and remove the postagecontract.LookupERC20Address function.

type ChainConfig struct {
	// General.
	ChainID                int64
	NetworkID              uint64
	PostageStampStartBlock uint64
	NativeTokenSymbol      string
	SwarmTokenSymbol       string

	// Addresses.
	StakingAddress         common.Address
	PostageStampAddress    common.Address
	RedistributionAddress  common.Address
	SwapPriceOracleAddress common.Address // Swap swear and swindle (S3) Contracts
	CurrentFactoryAddress  common.Address

	// ABIs.
	StakingABI        string
	PostageStampABI   string
	RedistributionABI string
}

var (
	Testnet = ChainConfig{
		ChainID:                abi.TestnetChainID,
		NetworkID:              abi.TestnetNetworkID,
		PostageStampStartBlock: abi.TestnetPostageStampBlockNumber,
		NativeTokenSymbol:      "ETH",
		SwarmTokenSymbol:       "sBZZ",

		StakingAddress:         common.HexToAddress(abi.TestnetStakingAddress),
		PostageStampAddress:    common.HexToAddress(abi.TestnetPostageStampAddress),
		RedistributionAddress:  common.HexToAddress(abi.TestnetRedistributionAddress),
		SwapPriceOracleAddress: common.HexToAddress("0x1814e9b3951Df0CB8e12b2bB99c5594514588936"),
		CurrentFactoryAddress:  common.HexToAddress("0x0fF044F6bB4F684a5A149B46D7eC03ea659F98A1"),

		StakingABI:        abi.TestnetStakingABI,
		PostageStampABI:   abi.TestnetPostageStampABI,
		RedistributionABI: abi.TestnetRedistributionABI,
	}

	Mainnet = ChainConfig{
		ChainID:                abi.MainnetChainID,
		NetworkID:              abi.MainnetNetworkID,
		PostageStampStartBlock: abi.MainnetPostageStampBlockNumber,
		NativeTokenSymbol:      "xDAI",
		SwarmTokenSymbol:       "xBZZ",

		StakingAddress:         common.HexToAddress(abi.MainnetStakingAddress),
		PostageStampAddress:    common.HexToAddress(abi.MainnetPostageStampAddress),
		RedistributionAddress:  common.HexToAddress(abi.MainnetRedistributionAddress),
		SwapPriceOracleAddress: common.HexToAddress("0xA57A50a831B31c904A770edBCb706E03afCdbd94"),
		CurrentFactoryAddress:  common.HexToAddress("0xc2d5a532cf69aa9a1378737d8ccdef884b6e7420"),

		StakingABI:        abi.MainnetStakingABI,
		PostageStampABI:   abi.MainnetPostageStampABI,
		RedistributionABI: abi.MainnetRedistributionABI,
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
			NativeTokenSymbol: Testnet.NativeTokenSymbol,
			SwarmTokenSymbol:  Testnet.SwarmTokenSymbol,
			StakingABI:        abi.TestnetStakingABI,
			PostageStampABI:   abi.TestnetPostageStampABI,
			RedistributionABI: abi.TestnetRedistributionABI,
		}, false
	}
}
