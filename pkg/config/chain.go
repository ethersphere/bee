// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config

import (
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/go-storage-incentives-abi/abi"
)

// ContractVersion describes a contract address and ABI that become active
// at the given redistribution round.
type ContractVersion struct {
	Address   common.Address
	ABI       string
	FromRound uint64
}

// ErrInvalidContractVersions is returned when a list of contract versions is malformed.
var ErrInvalidContractVersions = errors.New("config: invalid contract versions")

// ValidateContractVersions checks that the list is non-empty, starts at round 0,
// has strictly increasing FromRound values and that every version after the
// first has a non-zero address. A zero address on the first version is allowed
// (unknown chains).
func ValidateContractVersions(vs []ContractVersion) error {
	if len(vs) == 0 {
		return fmt.Errorf("%w: empty list", ErrInvalidContractVersions)
	}
	if vs[0].FromRound != 0 {
		return fmt.Errorf("%w: first version must start at round 0", ErrInvalidContractVersions)
	}
	for i := 1; i < len(vs); i++ {
		if vs[i].FromRound <= vs[i-1].FromRound {
			return fmt.Errorf("%w: from_round must be strictly increasing at index %d", ErrInvalidContractVersions, i)
		}
		if vs[i].Address == (common.Address{}) {
			return fmt.Errorf("%w: zero address at index %d", ErrInvalidContractVersions, i)
		}
	}
	return nil
}

type ChainConfig struct {
	// General.
	ChainID                int64
	NetworkID              uint64
	PostageStampStartBlock uint64
	NativeTokenSymbol      string
	SwarmTokenSymbol       string

	// Addresses.
	SwapPriceOracleAddress common.Address // Swap swear and swindle (S3) Contracts
	CurrentFactoryAddress  common.Address
	TokenContractAddress   common.Address

	// Versioned contracts. Each list is ordered by FromRound, starts at
	// round 0 and holds at least one entry.
	StakingVersions        []ContractVersion
	PostageStampVersions   []ContractVersion
	RedistributionVersions []ContractVersion

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

		SwapPriceOracleAddress: common.HexToAddress("0x1814e9b3951Df0CB8e12b2bB99c5594514588936"),
		CurrentFactoryAddress:  common.HexToAddress("0x0fF044F6bB4F684a5A149B46D7eC03ea659F98A1"),
		TokenContractAddress:   common.HexToAddress(abi.TestnetBzzTokenAddress),

		StakingVersions:        []ContractVersion{{Address: common.HexToAddress(abi.TestnetStakingAddress), ABI: abi.TestnetStakingABI}},
		PostageStampVersions:   []ContractVersion{{Address: common.HexToAddress(abi.TestnetPostageStampAddress), ABI: abi.TestnetPostageStampABI}},
		RedistributionVersions: []ContractVersion{{Address: common.HexToAddress(abi.TestnetRedistributionAddress), ABI: abi.TestnetRedistributionABI}},

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

		SwapPriceOracleAddress: common.HexToAddress("0xA57A50a831B31c904A770edBCb706E03afCdbd94"),
		CurrentFactoryAddress:  common.HexToAddress("0xc2d5a532cf69aa9a1378737d8ccdef884b6e7420"),
		TokenContractAddress:   common.HexToAddress(abi.MainnetBzzTokenAddress),

		StakingVersions:        []ContractVersion{{Address: common.HexToAddress(abi.MainnetStakingAddress), ABI: abi.MainnetStakingABI}},
		PostageStampVersions:   []ContractVersion{{Address: common.HexToAddress(abi.MainnetPostageStampAddress), ABI: abi.MainnetPostageStampABI}},
		RedistributionVersions: []ContractVersion{{Address: common.HexToAddress(abi.MainnetRedistributionAddress), ABI: abi.MainnetRedistributionABI}},

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
			NativeTokenSymbol:      Testnet.NativeTokenSymbol,
			SwarmTokenSymbol:       Testnet.SwarmTokenSymbol,
			StakingVersions:        []ContractVersion{{ABI: abi.TestnetStakingABI}},
			PostageStampVersions:   []ContractVersion{{ABI: abi.TestnetPostageStampABI}},
			RedistributionVersions: []ContractVersion{{ABI: abi.TestnetRedistributionABI}},
		}, false
	}
}
