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
			// Upgraders relying on the old chequebook-enable=true default must
			// now fail loudly instead of silently degrading to pseudo-settle.
			name: "legacy full-node true without chequebook fails",
			config: map[string]any{
				optionNameFullNode:                true,
				configKeyBlockchainRpcEndpoint:    "http://localhost:8545",
				optionNameSwapEnable:              true,
				optionNameStorageIncentivesEnable: true,
			},
			wantErr: "full node requires chequebook-enable",
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
