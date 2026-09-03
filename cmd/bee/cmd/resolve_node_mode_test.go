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

const testRPCEndpoint = "http://localhost:8545"

func TestResolveNodeMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		config   map[string]any
		wantMode node.NodeMode
		wantErr  string
		// wantOptions holds the values the resolver must leave in config for the
		// rest of startup to read. Keys not listed are not checked.
		wantOptions map[string]bool
	}{
		// ── node-mode set: the mode owns the config ──────────────────────────────
		{
			name: "full with rpc only implies swap, chequebook and incentives",
			config: map[string]any{
				optionNameNodeMode:             "full",
				configKeyBlockchainRpcEndpoint: testRPCEndpoint,
			},
			wantMode: node.FullMode,
			wantOptions: map[string]bool{
				optionNameSwapEnable:              true,
				optionNameChequebookEnable:        true,
				optionNameStorageIncentivesEnable: true,
			},
		},
		{
			name: "full with all options explicitly enabled succeeds",
			config: map[string]any{
				optionNameNodeMode:                "full",
				configKeyBlockchainRpcEndpoint:    testRPCEndpoint,
				optionNameSwapEnable:              true,
				optionNameChequebookEnable:        true,
				optionNameStorageIncentivesEnable: true,
			},
			wantMode: node.FullMode,
		},
		{
			name: "full without rpc fails",
			config: map[string]any{
				optionNameNodeMode: "full",
			},
			wantErr: "full node requires blockchain-rpc-endpoint",
		},
		{
			// A non-staking full node is a legitimate opt-out.
			name: "full with storage-incentives explicitly false honours the opt-out",
			config: map[string]any{
				optionNameNodeMode:                "full",
				configKeyBlockchainRpcEndpoint:    testRPCEndpoint,
				optionNameStorageIncentivesEnable: false,
			},
			wantMode: node.FullMode,
			wantOptions: map[string]bool{
				optionNameSwapEnable:              true,
				optionNameChequebookEnable:        true,
				optionNameStorageIncentivesEnable: false,
			},
		},
		{
			// Disabling swap must not drag an implied chequebook into a
			// contradiction the operator never wrote.
			name: "full with swap explicitly false leaves chequebook off",
			config: map[string]any{
				optionNameNodeMode:             "full",
				configKeyBlockchainRpcEndpoint: testRPCEndpoint,
				optionNameSwapEnable:           false,
			},
			wantMode: node.FullMode,
			wantOptions: map[string]bool{
				optionNameSwapEnable:              false,
				optionNameChequebookEnable:        false,
				optionNameStorageIncentivesEnable: true,
			},
		},
		{
			// Receive-only swap: cash out cheques without issuing them.
			name: "full with chequebook explicitly false keeps swap on",
			config: map[string]any{
				optionNameNodeMode:             "full",
				configKeyBlockchainRpcEndpoint: testRPCEndpoint,
				optionNameChequebookEnable:     false,
			},
			wantMode: node.FullMode,
			wantOptions: map[string]bool{
				optionNameSwapEnable:       true,
				optionNameChequebookEnable: false,
			},
		},
		{
			name: "full with chequebook explicitly true and swap explicitly false fails",
			config: map[string]any{
				optionNameNodeMode:             "full",
				configKeyBlockchainRpcEndpoint: testRPCEndpoint,
				optionNameSwapEnable:           false,
				optionNameChequebookEnable:     true,
			},
			wantErr: "chequebook-enable requires swap-enable",
		},
		{
			// NewBee never starts swap, push-sync or the incentives agent for a
			// bootnode, so full mode must not imply them there.
			name: "full bootnode does not imply swap, chequebook or incentives",
			config: map[string]any{
				optionNameNodeMode:             "full",
				optionNameBootnodeMode:         true,
				configKeyBlockchainRpcEndpoint: testRPCEndpoint,
			},
			wantMode: node.FullMode,
			wantOptions: map[string]bool{
				optionNameSwapEnable:              false,
				optionNameChequebookEnable:        false,
				optionNameStorageIncentivesEnable: false,
			},
		},
		{
			name: "full bootnode with storage-incentives explicitly false succeeds",
			config: map[string]any{
				optionNameNodeMode:                "full",
				optionNameBootnodeMode:            true,
				configKeyBlockchainRpcEndpoint:    testRPCEndpoint,
				optionNameStorageIncentivesEnable: false,
			},
			wantMode: node.FullMode,
		},
		{
			name: "light with rpc succeeds without swap",
			config: map[string]any{
				optionNameNodeMode:             "light",
				configKeyBlockchainRpcEndpoint: testRPCEndpoint,
			},
			wantMode: node.LightMode,
			wantOptions: map[string]bool{
				optionNameSwapEnable:              false,
				optionNameChequebookEnable:        false,
				optionNameStorageIncentivesEnable: false,
			},
		},
		{
			name: "light with rpc, swap and chequebook succeeds",
			config: map[string]any{
				optionNameNodeMode:             "light",
				configKeyBlockchainRpcEndpoint: testRPCEndpoint,
				optionNameSwapEnable:           true,
				optionNameChequebookEnable:     true,
			},
			wantMode: node.LightMode,
		},
		{
			name: "light without rpc fails",
			config: map[string]any{
				optionNameNodeMode: "light",
			},
			wantErr: "light node requires blockchain-rpc-endpoint",
		},
		{
			name: "light with chequebook but no swap fails",
			config: map[string]any{
				optionNameNodeMode:             "light",
				configKeyBlockchainRpcEndpoint: testRPCEndpoint,
				optionNameChequebookEnable:     true,
			},
			wantErr: "chequebook-enable requires swap-enable",
		},
		{
			name: "light rejects storage-incentives-enable",
			config: map[string]any{
				optionNameNodeMode:                "light",
				configKeyBlockchainRpcEndpoint:    testRPCEndpoint,
				optionNameStorageIncentivesEnable: true,
			},
			wantErr: "light node cannot have storage-incentives-enable",
		},
		{
			name: "ultra-light succeeds",
			config: map[string]any{
				optionNameNodeMode: "ultra-light",
			},
			wantMode: node.UltraLightMode,
		},
		{
			name: "ultra-light with rpc ignores rpc and succeeds",
			config: map[string]any{
				optionNameNodeMode:             "ultra-light",
				configKeyBlockchainRpcEndpoint: testRPCEndpoint,
			},
			wantMode: node.UltraLightMode,
		},
		{
			name: "ultra-light rejects swap-enable",
			config: map[string]any{
				optionNameNodeMode:   "ultra-light",
				optionNameSwapEnable: true,
			},
			wantErr: "ultra-light node cannot have swap-enable",
		},
		{
			name: "ultra-light rejects storage-incentives-enable",
			config: map[string]any{
				optionNameNodeMode:                "ultra-light",
				optionNameStorageIncentivesEnable: true,
			},
			wantErr: "ultra-light node cannot have storage-incentives-enable",
		},
		{
			name: "node-mode takes precedence over legacy full-node",
			config: map[string]any{
				optionNameNodeMode:             "light",
				optionNameFullNode:             true,
				configKeyBlockchainRpcEndpoint: testRPCEndpoint,
			},
			wantMode: node.LightMode,
		},
		{
			name: "invalid node-mode value fails",
			config: map[string]any{
				optionNameNodeMode: "superlight",
			},
			wantErr: "invalid node-mode",
		},
		{
			name: "uppercase node-mode fails",
			config: map[string]any{
				optionNameNodeMode: "FULL",
			},
			wantErr: "invalid node-mode",
		},
		{
			name: "whitespace node-mode fails",
			config: map[string]any{
				optionNameNodeMode: " full ",
			},
			wantErr: "invalid node-mode",
		},

		// ── node-mode unset: legacy behaviour, verbatim ─────────────────────────
		{
			// The most common pre-node-mode light config. chequebook-enable used
			// to default to true, so this node issued cheques; it must keep doing so.
			name: "legacy light with swap only restores chequebook default",
			config: map[string]any{
				configKeyBlockchainRpcEndpoint: testRPCEndpoint,
				optionNameSwapEnable:           true,
			},
			wantMode: node.LightMode,
			wantOptions: map[string]bool{
				optionNameSwapEnable:              true,
				optionNameChequebookEnable:        true,
				optionNameStorageIncentivesEnable: true,
			},
		},
		{
			// The old shipped default; chequebook stays gated on swap in NewBee.
			name: "legacy chequebook without swap starts",
			config: map[string]any{
				optionNameChequebookEnable: true,
			},
			wantMode: node.UltraLightMode,
		},
		{
			name: "legacy full-node with rpc only restores old defaults and leaves swap off",
			config: map[string]any{
				optionNameFullNode:             true,
				configKeyBlockchainRpcEndpoint: testRPCEndpoint,
			},
			wantMode: node.FullMode,
			wantOptions: map[string]bool{
				optionNameSwapEnable:              false,
				optionNameChequebookEnable:        true,
				optionNameStorageIncentivesEnable: true,
			},
		},
		{
			name: "legacy full-node with all options set maps to full",
			config: map[string]any{
				optionNameFullNode:                true,
				configKeyBlockchainRpcEndpoint:    testRPCEndpoint,
				optionNameSwapEnable:              true,
				optionNameChequebookEnable:        true,
				optionNameStorageIncentivesEnable: true,
			},
			wantMode: node.FullMode,
		},
		{
			name: "legacy full-node with explicit opt-outs is not validated",
			config: map[string]any{
				optionNameFullNode:                true,
				configKeyBlockchainRpcEndpoint:    testRPCEndpoint,
				optionNameSwapEnable:              false,
				optionNameChequebookEnable:        false,
				optionNameStorageIncentivesEnable: false,
			},
			wantMode: node.FullMode,
			wantOptions: map[string]bool{
				optionNameSwapEnable:              false,
				optionNameChequebookEnable:        false,
				optionNameStorageIncentivesEnable: false,
			},
		},
		{
			name: "legacy bootnode with storage-incentives false starts",
			config: map[string]any{
				optionNameFullNode:                true,
				optionNameBootnodeMode:            true,
				configKeyBlockchainRpcEndpoint:    testRPCEndpoint,
				optionNameStorageIncentivesEnable: false,
			},
			wantMode: node.FullMode,
		},
		{
			// Previous releases enabled the chain backend for every full node and
			// failed at chain init without an endpoint; keep failing, earlier.
			name: "legacy full-node without rpc fails",
			config: map[string]any{
				optionNameFullNode: true,
			},
			wantErr: "full node requires blockchain-rpc-endpoint",
		},
		{
			name: "legacy with rpc infers light",
			config: map[string]any{
				configKeyBlockchainRpcEndpoint: testRPCEndpoint,
			},
			wantMode: node.LightMode,
		},
		{
			name:     "legacy without rpc infers ultra-light",
			config:   map[string]any{},
			wantMode: node.UltraLightMode,
		},
		{
			// Beekeeper's inherited-config scenario: swap-enable inherited from the
			// base profile on a node without rpc. Legacy must not validate it.
			name: "legacy without rpc but with swap infers ultra-light without error",
			config: map[string]any{
				optionNameSwapEnable: true,
			},
			wantMode: node.UltraLightMode,
		},
		{
			// Legacy must not restore a default the operator overrode.
			name: "legacy explicit false is preserved",
			config: map[string]any{
				configKeyBlockchainRpcEndpoint:    testRPCEndpoint,
				optionNameSwapEnable:              true,
				optionNameChequebookEnable:        false,
				optionNameStorageIncentivesEnable: false,
			},
			wantMode: node.LightMode,
			wantOptions: map[string]bool{
				optionNameChequebookEnable:        false,
				optionNameStorageIncentivesEnable: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

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
			for key, want := range tt.wantOptions {
				if got := c.config.GetBool(key); got != want {
					t.Errorf("option %q: got %t, want %t", key, got, want)
				}
			}
		})
	}
}

// TestResolveNodeModeWithBoundFlags runs the resolver against a viper bound to
// the real start command flags, as production does, so the flag defaults are
// exercised: an unset node-mode must select the legacy regime, and the flag
// defaults of the sub-options must not count as explicitly set.
func TestResolveNodeModeWithBoundFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		args        []string
		wantMode    node.NodeMode
		wantOptions map[string]bool
	}{
		{
			name:     "no flags selects legacy regime and restores old defaults",
			args:     nil,
			wantMode: node.UltraLightMode,
			wantOptions: map[string]bool{
				optionNameChequebookEnable:        true,
				optionNameStorageIncentivesEnable: true,
			},
		},
		{
			name:     "legacy full-node flag with rpc",
			args:     []string{"--full-node", "--blockchain-rpc-endpoint=" + testRPCEndpoint},
			wantMode: node.FullMode,
			wantOptions: map[string]bool{
				optionNameSwapEnable:              false,
				optionNameChequebookEnable:        true,
				optionNameStorageIncentivesEnable: true,
			},
		},
		{
			name:     "node-mode full with rpc implies the full stack",
			args:     []string{"--node-mode=full", "--blockchain-rpc-endpoint=" + testRPCEndpoint},
			wantMode: node.FullMode,
			wantOptions: map[string]bool{
				optionNameSwapEnable:              true,
				optionNameChequebookEnable:        true,
				optionNameStorageIncentivesEnable: true,
			},
		},
		{
			name:     "node-mode light with rpc keeps sub-option defaults",
			args:     []string{"--node-mode=light", "--blockchain-rpc-endpoint=" + testRPCEndpoint},
			wantMode: node.LightMode,
			wantOptions: map[string]bool{
				optionNameSwapEnable:              false,
				optionNameChequebookEnable:        false,
				optionNameStorageIncentivesEnable: false,
			},
		},
		{
			name:     "node-mode full with incentives explicitly disabled",
			args:     []string{"--node-mode=full", "--blockchain-rpc-endpoint=" + testRPCEndpoint, "--storage-incentives-enable=false"},
			wantMode: node.FullMode,
			wantOptions: map[string]bool{
				optionNameSwapEnable:              true,
				optionNameChequebookEnable:        true,
				optionNameStorageIncentivesEnable: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root, err := newCommand(func(c *command) { c.homeDir = t.TempDir() })
			if err != nil {
				t.Fatal(err)
			}
			startCmd := root.SubCommandForTest("start")
			if startCmd == nil {
				t.Fatal("start subcommand not found")
			}
			if err := startCmd.ParseFlags(tt.args); err != nil {
				t.Fatal(err)
			}

			// Mirror the start command's PreRunE: bind flags, then map the flat
			// blockchain-rpc-* flags onto their nested config keys.
			c := &command{
				config: viper.New(),
				logger: log.Noop,
			}
			if err := c.config.BindPFlags(startCmd.Flags()); err != nil {
				t.Fatal(err)
			}
			c.bindBlockchainRpcConfig(startCmd)

			gotMode, err := c.resolveNodeMode(c.logger)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotMode != tt.wantMode {
				t.Errorf("got mode %q, want %q", gotMode, tt.wantMode)
			}
			for key, want := range tt.wantOptions {
				if got := c.config.GetBool(key); got != want {
					t.Errorf("option %q: got %t, want %t", key, got, want)
				}
			}
		})
	}
}
