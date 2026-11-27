// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node_test

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/config"
	"github.com/ethersphere/bee/v2/pkg/node"
)

func TestApplyAddressOverride(t *testing.T) {
	t.Parallel()

	a0 := common.HexToAddress("0x1")
	a1 := common.HexToAddress("0x2")
	ov := common.HexToAddress("0x3")
	in := []config.ContractVersion{
		{Address: a0, ABI: "abi0", FromRound: 0},
		{Address: a1, ABI: "abi1", FromRound: 5},
	}

	t.Run("empty override", func(t *testing.T) {
		t.Parallel()
		out, err := node.ApplyAddressOverride(in, "", "staking contract")
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 2 || out[0] != in[0] || out[1] != in[1] {
			t.Fatalf("unexpected result %v", out)
		}
	})

	t.Run("valid override", func(t *testing.T) {
		t.Parallel()
		out, err := node.ApplyAddressOverride(in, ov.Hex(), "staking contract")
		if err != nil {
			t.Fatal(err)
		}
		if out[0].Address != ov || out[0].ABI != "abi0" || out[0].FromRound != 0 {
			t.Fatalf("unexpected first version %v", out[0])
		}
		if out[1] != in[1] {
			t.Fatalf("second version changed: %v", out[1])
		}
		if in[0].Address != a0 {
			t.Fatal("input slice mutated")
		}
	})

	t.Run("malformed override", func(t *testing.T) {
		t.Parallel()
		_, err := node.ApplyAddressOverride(in, "nothex", "staking contract")
		if err == nil || !strings.Contains(err.Error(), "staking contract") {
			t.Fatalf("unexpected error %v", err)
		}
	})
}

func TestFromBlock(t *testing.T) {
	t.Parallel()

	if got := node.FromBlock(2); got != 304 {
		t.Fatalf("got %d, want 304", got)
	}
	if got := node.FromBlock(0); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}
