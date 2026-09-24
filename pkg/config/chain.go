// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	_ "embed"
	"encoding/hex"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/go-storage-incentives-abi/abi"
)

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
	TokenContractAddress   common.Address

	// ABIs.
	StakingABI        string
	PostageStampABI   string
	RedistributionABI string

	// AcceptedChequebookBytecodeHashes is the set of Keccak256(eth_getCode)
	// hashes that the chequebook verifier accepts. One entry per factory
	// generation. Append-never-remove: removing a hash would reject
	// chequebooks deployed by that factory generation.
	// Derive: cast keccak $(cast code <any chequebook addr> --rpc-url <rpc>)
	AcceptedChequebookBytecodeHashes [][32]byte
}

// mustHash decodes a 64-character hex string (no 0x prefix) into a [32]byte.
// Panics on malformed input so misconfigured hashes are caught at startup.
func mustHash(hexStr string) [32]byte {
	b, err := hex.DecodeString(hexStr)
	if err != nil || len(b) != 32 {
		panic("config: invalid 32-byte hex hash: " + hexStr)
	}
	return [32]byte(b)
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
		TokenContractAddress:   common.HexToAddress(abi.TestnetBzzTokenAddress),

		StakingABI:        abi.TestnetStakingABI,
		PostageStampABI:   abi.TestnetPostageStampABI,
		RedistributionABI: abi.TestnetRedistributionABI,

		AcceptedChequebookBytecodeHashes: [][32]byte{
			mustHash("ba50aa67c6e6f135a8ca57947c015c24192531d47e47a9ec212c0090e0486d46"),
		},
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
		TokenContractAddress:   common.HexToAddress(abi.MainnetBzzTokenAddress),

		StakingABI:        abi.MainnetStakingABI,
		PostageStampABI:   abi.MainnetPostageStampABI,
		RedistributionABI: abi.MainnetRedistributionABI,

		AcceptedChequebookBytecodeHashes: [][32]byte{
			mustHash("81d3de06cadb0970fc653f24cef4689243f9a3d702236370ecf4613673048145"),
		},
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
