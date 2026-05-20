// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/config"
	"github.com/ethersphere/bee/v2/pkg/node"
	"github.com/ethersphere/bee/v2/pkg/transaction"
)

// These tests cover setupPostageContract: it resolves the postage stamp
// contract address (default-from-chain-config vs custom override), parses the
// ABI, and calls LookupERC20Address. Validation of malformed addresses /
// missing start blocks already lives in validateChainContractOptions, so the
// happy paths and the LookupERC20 error are the interesting cases here.

func mockPostageDeps(t *testing.T, want common.Address, wantErr error, lookupCalls *int, observed *common.Address) node.PostageContractDeps {
	t.Helper()
	return node.PostageContractDeps{
		LookupERC20: func(_ context.Context, _ transaction.Service, postageStampContractAddress common.Address, _ abi.ABI, _ bool) (common.Address, error) {
			*lookupCalls++
			if observed != nil {
				*observed = postageStampContractAddress
			}
			return want, wantErr
		},
	}
}

func TestSetupPostageContract_DefaultChainConfig_NoOverride(t *testing.T) {
	t.Parallel()

	wantBzz := common.HexToAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	var calls int
	var lookedUpAddr common.Address

	res, err := node.SetupPostageContract(
		context.Background(),
		&node.Options{}, // no custom postage address
		config.Mainnet.ChainID,
		true,
		nil, // transactionService — fake never uses it
		mockPostageDeps(t, wantBzz, nil, &calls, &lookedUpAddr),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("LookupERC20 should run exactly once, got %d", calls)
	}
	if res.BzzTokenAddress != wantBzz {
		t.Fatalf("BzzTokenAddress: got %s, want %s", res.BzzTokenAddress, wantBzz)
	}
	// Without a custom override, the address fed to LookupERC20 must be the
	// chain's default postage stamp address.
	if lookedUpAddr != config.Mainnet.PostageStampAddress {
		t.Fatalf("looked up addr: got %s, want %s (chain default)", lookedUpAddr, config.Mainnet.PostageStampAddress)
	}
	if res.ContractAddress != config.Mainnet.PostageStampAddress {
		t.Fatalf("ContractAddress: got %s, want %s", res.ContractAddress, config.Mainnet.PostageStampAddress)
	}
	if res.SyncStartBlock != config.Mainnet.PostageStampStartBlock {
		t.Fatalf("SyncStartBlock: got %d, want %d", res.SyncStartBlock, config.Mainnet.PostageStampStartBlock)
	}
	if res.ChainConfig.ChainID != config.Mainnet.ChainID {
		t.Fatalf("ChainConfig.ChainID: got %d, want %d", res.ChainConfig.ChainID, config.Mainnet.ChainID)
	}
}

func TestSetupPostageContract_CustomOverride_UsedInLookup(t *testing.T) {
	t.Parallel()

	customAddr := "0x1234567890123456789012345678901234567890"
	customStart := uint64(42)
	wantBzz := common.HexToAddress("0xcccccccccccccccccccccccccccccccccccccccc")
	var calls int
	var lookedUpAddr common.Address

	res, err := node.SetupPostageContract(
		context.Background(),
		&node.Options{
			PostageContractAddress:    customAddr,
			PostageContractStartBlock: customStart,
		},
		config.Mainnet.ChainID,
		true,
		nil,
		mockPostageDeps(t, wantBzz, nil, &calls, &lookedUpAddr),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("LookupERC20 should run exactly once, got %d", calls)
	}
	wantCustom := common.HexToAddress(customAddr)
	if lookedUpAddr != wantCustom {
		t.Fatalf("LookupERC20 must receive the custom address, got %s, want %s", lookedUpAddr, wantCustom)
	}
	if res.ContractAddress != wantCustom {
		t.Fatalf("ContractAddress: got %s, want %s", res.ContractAddress, wantCustom)
	}
	if res.SyncStartBlock != customStart {
		t.Fatalf("SyncStartBlock: got %d, want %d", res.SyncStartBlock, customStart)
	}
}

func TestSetupPostageContract_LookupError_IsWrapped(t *testing.T) {
	t.Parallel()

	want := errors.New("rpc say no")
	var calls int
	res, err := node.SetupPostageContract(
		context.Background(),
		&node.Options{},
		config.Mainnet.ChainID,
		true,
		nil,
		mockPostageDeps(t, common.Address{}, want, &calls, nil),
	)
	if !errors.Is(err, want) {
		t.Fatalf("expected wrapped %v, got %v", want, err)
	}
	if !strings.Contains(err.Error(), "lookup erc20 postage address") {
		t.Fatalf("expected error to start with %q, got %q", "lookup erc20 postage address", err.Error())
	}
	// On error the result is a zero-value; nothing should leak through.
	if (res.BzzTokenAddress != common.Address{}) || (res.ContractAddress != common.Address{}) {
		t.Fatalf("expected zero result on error, got %+v", res)
	}
}

func TestSetupPostageContract_ChainEnabledFlag_PropagatesToLookup(t *testing.T) {
	t.Parallel()

	var lastChainEnabled bool
	deps := node.PostageContractDeps{
		LookupERC20: func(_ context.Context, _ transaction.Service, _ common.Address, _ abi.ABI, chainEnabled bool) (common.Address, error) {
			lastChainEnabled = chainEnabled
			return common.Address{}, nil
		},
	}

	for _, want := range []bool{true, false} {
		if _, err := node.SetupPostageContract(
			context.Background(),
			&node.Options{},
			config.Mainnet.ChainID,
			want,
			nil,
			deps,
		); err != nil {
			t.Fatalf("chainEnabled=%v: unexpected error: %v", want, err)
		}
		if lastChainEnabled != want {
			t.Fatalf("chainEnabled flag must be forwarded to LookupERC20: got %v, want %v", lastChainEnabled, want)
		}
	}
}
