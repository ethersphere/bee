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

	BlocksPerRound uint64
	BlocksPerPhase uint64
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

		BlocksPerRound: 152,
		BlocksPerPhase: 4,
	}

	//github.com/ethersphere/go-storage-incentives-abi v0.9.4

	Gnosis = ChainConfig{
		ChainID:                abi.MainnetChainID,
		NetworkID:              abi.MainnetNetworkID,
		PostageStampStartBlock: abi.MainnetPostageStampBlockNumber,
		NativeTokenSymbol:      "xDAI",
		SwarmTokenSymbol:       "xBZZ",

		StakingAddress:         common.HexToAddress(abi.MainnetStakingAddress),        //0xda2a16EE889E7F04980A8d597b48c8D51B9518F4
		PostageStampAddress:    common.HexToAddress(abi.MainnetPostageStampAddress),   //0x45a1502382541Cd610CC9068e88727426b696293
		RedistributionAddress:  common.HexToAddress(abi.MainnetRedistributionAddress), //0x5069cdfB3D9E56d23B1cAeE83CE6109A7E4fd62d
		SwapPriceOracleAddress: common.HexToAddress("0xA57A50a831B31c904A770edBCb706E03afCdbd94"),
		CurrentFactoryAddress:  common.HexToAddress("0xc2d5a532cf69aa9a1378737d8ccdef884b6e7420"),

		StakingABI:        abi.MainnetStakingABI,
		PostageStampABI:   abi.MainnetPostageStampABI,
		RedistributionABI: abi.MainnetRedistributionABI,

		BlocksPerRound: 152,
		BlocksPerPhase: 4,
	}

	Base = ChainConfig{
		ChainID:                8453,
		NetworkID:              2,
		PostageStampStartBlock: 45333498, //41062386,
		NativeTokenSymbol:      "bETH",
		SwarmTokenSymbol:       "bBZZ",

		StakingAddress:         common.HexToAddress("0x491075e789DBdbb7d08D95946E665eFB2751eE1E"),
		PostageStampAddress:    common.HexToAddress("0x8613A18717E30be14852846eC6D45F5010339451"),
		RedistributionAddress:  common.HexToAddress("0x6a02826e2a56092F56e0ba4dB766c5f4540414C2"),
		SwapPriceOracleAddress: common.HexToAddress("0x4c90551763C1498aE96589202E386019655c1781"),
		CurrentFactoryAddress:  common.HexToAddress("0xe4620F49ebDEF146366E63B08Eb66cAe32d51c8f"),

		StakingABI:        abi.MainnetStakingABI,
		PostageStampABI:   abi.MainnetPostageStampABI,
		RedistributionABI: abi.MainnetRedistributionABI,

		BlocksPerRound: 380,
		BlocksPerPhase: 4,
	}
)

func GetByChainID(chainID int64) (ChainConfig, bool) {
	switch chainID {
	case Testnet.ChainID:
		return Testnet, true
	case Gnosis.ChainID:
		return Gnosis, true
	case Base.ChainID:
		return Base, true
	default:
		return ChainConfig{
			NativeTokenSymbol: Testnet.NativeTokenSymbol,
			SwarmTokenSymbol:  Testnet.SwarmTokenSymbol,
			StakingABI:        abi.TestnetStakingABI,
			PostageStampABI:   abi.TestnetPostageStampABI,
			RedistributionABI: abi.TestnetRedistributionABI,
			BlocksPerRound:    Testnet.BlocksPerRound,
			BlocksPerPhase:    Testnet.BlocksPerPhase,
		}, false
	}
}
