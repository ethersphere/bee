// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package node_test

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/node"
)

func TestResolveRedistributionAddress(t *testing.T) {
	t.Parallel()

	cfg := common.HexToAddress("0x1111111111111111111111111111111111111111")
	override := common.HexToAddress("0x2222222222222222222222222222222222222222")

	t.Run("config used when override empty", func(t *testing.T) {
		t.Parallel()

		got, err := node.ResolveRedistributionAddress(cfg, "")
		if err != nil {
			t.Fatal(err)
		}
		if got != cfg {
			t.Fatalf("got %s, want %s", got, cfg)
		}
	})

	t.Run("override wins over config", func(t *testing.T) {
		t.Parallel()

		got, err := node.ResolveRedistributionAddress(cfg, override.Hex())
		if err != nil {
			t.Fatal(err)
		}
		if got != override {
			t.Fatalf("got %s, want %s", got, override)
		}
	})

	t.Run("malformed override", func(t *testing.T) {
		t.Parallel()

		_, err := node.ResolveRedistributionAddress(cfg, "0x12")
		if err == nil || !strings.Contains(err.Error(), "malformed") {
			t.Fatalf("got %v, want malformed error", err)
		}
	})

	t.Run("no address known", func(t *testing.T) {
		t.Parallel()

		_, err := node.ResolveRedistributionAddress(common.Address{}, "")
		if err == nil || !strings.Contains(err.Error(), "--redistribution-address") {
			t.Fatalf("got %v, want missing address error", err)
		}
	})
}
