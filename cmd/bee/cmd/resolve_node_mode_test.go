// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"strings"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/node"
	"github.com/spf13/viper"
)

func TestResolveNodeMode(t *testing.T) {
	tests := []struct {
		name     string
		config   map[string]any
		wantMode node.NodeMode
		wantErr  string
	}{
		// ── Explicit node-mode: strict validation ────────────────────────────────
		{
			name: "full mode with rpc, swap, chequebook and incentives succeeds",
			config: map[string]any{
				optionNameNodeMode:                "full",
				configKeyBlockchainRpcEndpoint:    "http://localhost:8545",
				optionNameSwapEnable:              true,
				optionNameChequebookEnable:        true,
				optionNameStorageIncentivesEnable: true,
			},
			wantMode: node.FullMode,
		},
		{
			name: "full mode without rpc fails",
			config: map[string]any{
				optionNameNodeMode:   "full",
				optionNameSwapEnable: true,
			},
			wantErr: "full node requires blockchain-rpc-endpoint",
		},
		{
			name: "full mode without swap fails",
			config: map[string]any{
				optionNameNodeMode:             "full",
				configKeyBlockchainRpcEndpoint: "http://localhost:8545",
			},
			wantErr: "full node requires swap-enable",
		},
		{
			name: "full mode without chequebook fails",
			config: map[string]any{
				optionNameNodeMode:                "full",
				configKeyBlockchainRpcEndpoint:    "http://localhost:8545",
				optionNameSwapEnable:              true,
				optionNameStorageIncentivesEnable: true,
			},
			wantErr: "full node requires chequebook-enable",
		},
		{
			name: "full mode without storage-incentives fails",
			config: map[string]any{
				optionNameNodeMode:             "full",
				configKeyBlockchainRpcEndpoint: "http://localhost:8545",
				optionNameSwapEnable:           true,
				optionNameChequebookEnable:     true,
			},
			wantErr: "storage-incentives-enable",
		},
		{
			name: "chequebook-enable without swap-enable fails (light mode)",
			config: map[string]any{
				optionNameNodeMode:             "light",
				configKeyBlockchainRpcEndpoint: "http://localhost:8545",
				optionNameChequebookEnable:     true,
			},
			wantErr: "chequebook-enable requires swap-enable",
		},
		{
			name: "light mode with rpc succeeds",
			config: map[string]any{
				optionNameNodeMode:             "light",
				configKeyBlockchainRpcEndpoint: "http://localhost:8545",
			},
			wantMode: node.LightMode,
		},
		{
			name: "light mode without rpc fails",
			config: map[string]any{
				optionNameNodeMode: "light",
			},
			wantErr: "light node requires blockchain-rpc-endpoint",
		},
		{
			name: "ultra-light mode succeeds",
			config: map[string]any{
				optionNameNodeMode: "ultra-light",
			},
			wantMode: node.UltraLightMode,
		},
		{
			name: "ultra-light mode rejects swap-enable",
			config: map[string]any{
				optionNameNodeMode:   "ultra-light",
				optionNameSwapEnable: true,
			},
			wantErr: "ultra-light node cannot have swap-enable",
		},
		{
			name: "invalid node-mode value fails",
			config: map[string]any{
				optionNameNodeMode: "superlight",
			},
			wantErr: "invalid node-mode",
		},

		// ── Legacy path: no node-mode set ────────────────────────────────────────
		{
			name: "legacy full-node true with all required flags maps to full mode",
			config: map[string]any{
				optionNameFullNode:                true,
				configKeyBlockchainRpcEndpoint:    "http://localhost:8545",
				optionNameSwapEnable:              true,
				optionNameChequebookEnable:        true,
				optionNameStorageIncentivesEnable: true,
			},
			wantMode: node.FullMode,
		},
		{
			// Upgrade compatibility: chequebook-enable and storage-incentives-enable
			// defaulted to true before node-mode existed. A legacy --full-node
			// config that did not set them explicitly is auto-enabled so the node
			// still starts as a fully-functional full node instead of failing
			// validation (no silent degradation).
			name: "legacy full-node true auto-enables chequebook and incentives",
			config: map[string]any{
				optionNameFullNode:             true,
				configKeyBlockchainRpcEndpoint: "http://localhost:8545",
				optionNameSwapEnable:           true,
			},
			wantMode: node.FullMode,
		},
		{
			// The compatibility default only restores the old implicit value; an
			// operator who explicitly disables chequebook while requesting a full
			// node has a genuine misconfiguration that must still fail loudly.
			name: "legacy full-node true with chequebook explicitly false fails",
			config: map[string]any{
				optionNameFullNode:             true,
				configKeyBlockchainRpcEndpoint: "http://localhost:8545",
				optionNameSwapEnable:           true,
				optionNameChequebookEnable:     false,
			},
			wantErr: "full node requires chequebook-enable",
		},
		{
			// Same: explicitly disabling storage incentives must not be masked.
			name: "legacy full-node true with storage-incentives explicitly false fails",
			config: map[string]any{
				optionNameFullNode:                true,
				configKeyBlockchainRpcEndpoint:    "http://localhost:8545",
				optionNameSwapEnable:              true,
				optionNameChequebookEnable:        true,
				optionNameStorageIncentivesEnable: false,
			},
			wantErr: "storage-incentives-enable",
		},
		{
			// swap-enable was also implied by --full-node before node-mode; a
			// legacy config with only the RPC endpoint set is fully restored and
			// starts as a full node rather than failing.
			name: "legacy full-node true with only rpc auto-enables full stack",
			config: map[string]any{
				optionNameFullNode:             true,
				configKeyBlockchainRpcEndpoint: "http://localhost:8545",
			},
			wantMode: node.FullMode,
		},
		{
			// The RPC endpoint is the one thing the compatibility default cannot
			// invent; a legacy full-node config without it must still fail.
			name: "legacy full-node true without rpc endpoint fails",
			config: map[string]any{
				optionNameFullNode: true,
			},
			wantErr: "full node requires blockchain-rpc-endpoint",
		},
		{
			name: "legacy with rpc endpoint infers light mode",
			config: map[string]any{
				configKeyBlockchainRpcEndpoint: "http://localhost:8545",
			},
			wantMode: node.LightMode,
		},
		{
			name:     "legacy without rpc endpoint infers ultra-light mode",
			config:   map[string]any{},
			wantMode: node.UltraLightMode,
		},
		{
			// Beekeeper's inherited-config scenario: rpc + swap-enable without node-mode.
			// Legacy path must NOT apply strict swap validation; this was the CI regression.
			name: "legacy with rpc and swap-enable infers light without error",
			config: map[string]any{
				configKeyBlockchainRpcEndpoint: "http://localhost:8545",
				optionNameSwapEnable:           true,
			},
			wantMode: node.LightMode,
		},
		{
			// Same scenario but for ultra-light: no rpc, swap-enable inherited from base.
			name: "legacy without rpc but with swap-enable infers ultra-light without error",
			config: map[string]any{
				optionNameSwapEnable: true,
			},
			wantMode: node.UltraLightMode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &command{
				config: viper.New(),
				logger: log.Noop,
			}
			for k, v := range tt.config {
				c.config.Set(k, v)
			}

			gotMode, err := c.resolveNodeMode(c.logger)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (mode=%q)", tt.wantErr, gotMode)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotMode != tt.wantMode {
				t.Errorf("got mode %q, want %q", gotMode, tt.wantMode)
			}
		})
	}
}

// TestResolveNodeModeLegacyBackcompatWritesConfig verifies that the legacy
// --full-node compatibility path writes the restored swap-enable,
// chequebook-enable and storage-incentives-enable defaults back into the
// config, so the rest of node startup (which reads them directly from config)
// sees them enabled.
func TestResolveNodeModeLegacyBackcompatWritesConfig(t *testing.T) {
	c := &command{
		config: viper.New(),
		logger: log.Noop,
	}
	c.config.Set(optionNameFullNode, true)
	c.config.Set(configKeyBlockchainRpcEndpoint, "http://localhost:8545")

	mode, err := c.resolveNodeMode(c.logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != node.FullMode {
		t.Fatalf("got mode %q, want %q", mode, node.FullMode)
	}
	for _, key := range []string{
		optionNameSwapEnable,
		optionNameChequebookEnable,
		optionNameStorageIncentivesEnable,
	} {
		if !c.config.GetBool(key) {
			t.Errorf("expected %q to be written back as true", key)
		}
	}
}
