// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package config_test

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethersphere/go-storage-incentives-abi/abi"

	"github.com/ethersphere/bee/v2/pkg/config"
)

func TestChainConfigTokenContractAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  config.ChainConfig
		want common.Address
	}{
		{
			name: "mainnet",
			cfg:  config.Mainnet,
			want: common.HexToAddress(abi.MainnetBzzTokenAddress),
		},
		{
			name: "testnet",
			cfg:  config.Testnet,
			want: common.HexToAddress(abi.TestnetBzzTokenAddress),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if (tc.want == common.Address{}) {
				t.Fatal("expected a non-zero bzz token address")
			}
			if tc.cfg.TokenContractAddress != tc.want {
				t.Fatalf("got token contract address %s, want %s", tc.cfg.TokenContractAddress, tc.want)
			}
		})
	}
}

func TestGetByChainIDTokenContractAddress(t *testing.T) {
	t.Parallel()

	t.Run("known chain", func(t *testing.T) {
		t.Parallel()

		cfg, found := config.GetByChainID(config.Mainnet.ChainID)
		if !found {
			t.Fatal("expected mainnet to be a known chain")
		}
		if cfg.TokenContractAddress != common.HexToAddress(abi.MainnetBzzTokenAddress) {
			t.Fatalf("got token contract address %s, want %s", cfg.TokenContractAddress, abi.MainnetBzzTokenAddress)
		}
	})

	t.Run("unknown chain", func(t *testing.T) {
		t.Parallel()

		cfg, found := config.GetByChainID(-1)
		if found {
			t.Fatal("expected unknown chain to be reported as not found")
		}
		if (cfg.TokenContractAddress != common.Address{}) {
			t.Fatalf("expected zero token contract address for unknown chain, got %s", cfg.TokenContractAddress)
		}
	})
}

// TestDeriveChequebookBytecodeHash derives the keccak256(eth_getCode) hash for
// a known-good chequebook deployed by a factory. Run this manually after a
// factory upgrade to get the new hash for AcceptedChequebookBytecodeHashes.
//
// Usage:
//
//	RPC_URL=https://... CHEQUEBOOK_ADDR=0x... go test ./pkg/config/... -run TestDeriveChequebookBytecodeHash -v
func TestDeriveChequebookBytecodeHash(t *testing.T) {
	rpcURL := os.Getenv("RPC_URL")
	addrStr := os.Getenv("CHEQUEBOOK_ADDR")
	if rpcURL == "" || addrStr == "" {
		t.Skip("set RPC_URL and CHEQUEBOOK_ADDR to run")
	}

	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		t.Fatalf("dial %s: %v", rpcURL, err)
	}
	defer client.Close()

	addr := common.HexToAddress(addrStr)
	code, err := client.CodeAt(context.Background(), addr, nil)
	if err != nil {
		t.Fatalf("CodeAt %s: %v", addr, err)
	}
	if len(code) == 0 {
		t.Fatalf("no deployed code at %s", addr)
	}

	hash := crypto.Keccak256Hash(code)
	t.Logf("add to AcceptedChequebookBytecodeHashes in pkg/config/chain.go:")
	t.Logf("\tmustHash(%q)", hex.EncodeToString(hash[:]))
}

func TestValidateContractVersions(t *testing.T) {
	t.Parallel()

	a1 := common.HexToAddress("0x01")
	a2 := common.HexToAddress("0x02")

	tests := []struct {
		name    string
		vs      []config.ContractVersion
		wantErr bool
	}{
		{name: "nil", vs: nil, wantErr: true},
		{name: "first not at round zero", vs: []config.ContractVersion{{FromRound: 1}}, wantErr: true},
		{name: "duplicate rounds", vs: []config.ContractVersion{{}, {FromRound: 5, Address: a1}, {FromRound: 5, Address: a2}}, wantErr: true},
		{name: "non-first zero address", vs: []config.ContractVersion{{}, {FromRound: 5}}, wantErr: true},
		{name: "valid two versions", vs: []config.ContractVersion{{Address: a1}, {FromRound: 5, Address: a2}}},
		{name: "first zero address allowed", vs: []config.ContractVersion{{}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := config.ValidateContractVersions(tc.vs)
			if tc.wantErr {
				if !errors.Is(err, config.ErrInvalidContractVersions) {
					t.Fatalf("got error %v, want ErrInvalidContractVersions", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestKnownChainContractVersionsValid(t *testing.T) {
	t.Parallel()

	for name, cfg := range map[string]config.ChainConfig{"mainnet": config.Mainnet, "testnet": config.Testnet} {
		for list, vs := range map[string][]config.ContractVersion{
			"staking":        cfg.StakingVersions,
			"postage":        cfg.PostageStampVersions,
			"redistribution": cfg.RedistributionVersions,
		} {
			if err := config.ValidateContractVersions(vs); err != nil {
				t.Fatalf("%s %s: %v", name, list, err)
			}
			if vs[0].Address == (common.Address{}) {
				t.Fatalf("%s %s: zero first address", name, list)
			}
		}
	}
}
