// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ethersphere/bee/v2/cmd/bee/cmd"
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
				cmd.OptionNameNodeMode:             "full",
				cmd.ConfigKeyBlockchainRpcEndpoint: testRPCEndpoint,
			},
			wantMode: node.FullMode,
			wantOptions: map[string]bool{
				cmd.OptionNameSwapEnable:              true,
				cmd.OptionNameChequebookEnable:        true,
				cmd.OptionNameStorageIncentivesEnable: true,
			},
		},
		{
			name: "full with all options explicitly enabled succeeds",
			config: map[string]any{
				cmd.OptionNameNodeMode:                "full",
				cmd.ConfigKeyBlockchainRpcEndpoint:    testRPCEndpoint,
				cmd.OptionNameSwapEnable:              true,
				cmd.OptionNameChequebookEnable:        true,
				cmd.OptionNameStorageIncentivesEnable: true,
			},
			wantMode: node.FullMode,
		},
		{
			name: "full without rpc is rejected",
			config: map[string]any{
				cmd.OptionNameNodeMode: "full",
			},
			wantErr: "full node requires blockchain-rpc-endpoint",
		},
		{
			// A non-staking full node is a legitimate opt-out.
			name: "full with storage-incentives explicitly false honors the opt-out",
			config: map[string]any{
				cmd.OptionNameNodeMode:                "full",
				cmd.ConfigKeyBlockchainRpcEndpoint:    testRPCEndpoint,
				cmd.OptionNameStorageIncentivesEnable: false,
			},
			wantMode: node.FullMode,
			wantOptions: map[string]bool{
				cmd.OptionNameSwapEnable:              true,
				cmd.OptionNameChequebookEnable:        true,
				cmd.OptionNameStorageIncentivesEnable: false,
			},
		},
		{
			// Disabling swap must not drag an implied chequebook into a
			// contradiction the operator never wrote.
			name: "full with swap explicitly false leaves chequebook off",
			config: map[string]any{
				cmd.OptionNameNodeMode:             "full",
				cmd.ConfigKeyBlockchainRpcEndpoint: testRPCEndpoint,
				cmd.OptionNameSwapEnable:           false,
			},
			wantMode: node.FullMode,
			wantOptions: map[string]bool{
				cmd.OptionNameSwapEnable:              false,
				cmd.OptionNameChequebookEnable:        false,
				cmd.OptionNameStorageIncentivesEnable: true,
			},
		},
		{
			// Receive-only swap: cash out cheques without issuing them.
			name: "full with chequebook explicitly false keeps swap on",
			config: map[string]any{
				cmd.OptionNameNodeMode:             "full",
				cmd.ConfigKeyBlockchainRpcEndpoint: testRPCEndpoint,
				cmd.OptionNameChequebookEnable:     false,
			},
			wantMode: node.FullMode,
			wantOptions: map[string]bool{
				cmd.OptionNameSwapEnable:       true,
				cmd.OptionNameChequebookEnable: false,
			},
		},
		{
			name: "full with chequebook explicitly true and swap explicitly false is rejected",
			config: map[string]any{
				cmd.OptionNameNodeMode:             "full",
				cmd.ConfigKeyBlockchainRpcEndpoint: testRPCEndpoint,
				cmd.OptionNameSwapEnable:           false,
				cmd.OptionNameChequebookEnable:     true,
			},
			wantErr: "chequebook-enable requires swap-enable",
		},
		{
			// NewBee never runs the reserve or the incentives agent for a
			// bootnode, so full mode must not imply swap, chequebook or
			// incentives there.
			name: "full bootnode does not imply swap, chequebook or incentives",
			config: map[string]any{
				cmd.OptionNameNodeMode:             "full",
				cmd.OptionNameBootnodeMode:         true,
				cmd.ConfigKeyBlockchainRpcEndpoint: testRPCEndpoint,
			},
			wantMode: node.FullMode,
			wantOptions: map[string]bool{
				cmd.OptionNameSwapEnable:              false,
				cmd.OptionNameChequebookEnable:        false,
				cmd.OptionNameStorageIncentivesEnable: false,
			},
		},
		{
			name: "full bootnode with swap explicitly enabled enables chequebook",
			config: map[string]any{
				cmd.OptionNameNodeMode:             "full",
				cmd.OptionNameBootnodeMode:         true,
				cmd.ConfigKeyBlockchainRpcEndpoint: testRPCEndpoint,
				cmd.OptionNameSwapEnable:           true,
			},
			wantMode: node.FullMode,
			wantOptions: map[string]bool{
				cmd.OptionNameSwapEnable:       true,
				cmd.OptionNameChequebookEnable: true,
			},
		},
		{
			name: "full bootnode with storage-incentives explicitly false succeeds",
			config: map[string]any{
				cmd.OptionNameNodeMode:                "full",
				cmd.OptionNameBootnodeMode:            true,
				cmd.ConfigKeyBlockchainRpcEndpoint:    testRPCEndpoint,
				cmd.OptionNameStorageIncentivesEnable: false,
			},
			wantMode: node.FullMode,
		},
		{
			name: "light with rpc succeeds without swap",
			config: map[string]any{
				cmd.OptionNameNodeMode:             "light",
				cmd.ConfigKeyBlockchainRpcEndpoint: testRPCEndpoint,
			},
			wantMode: node.LightMode,
			wantOptions: map[string]bool{
				cmd.OptionNameSwapEnable:              false,
				cmd.OptionNameChequebookEnable:        false,
				cmd.OptionNameStorageIncentivesEnable: false,
			},
		},
		{
			name: "light with rpc, swap and chequebook succeeds",
			config: map[string]any{
				cmd.OptionNameNodeMode:             "light",
				cmd.ConfigKeyBlockchainRpcEndpoint: testRPCEndpoint,
				cmd.OptionNameSwapEnable:           true,
				cmd.OptionNameChequebookEnable:     true,
			},
			wantMode: node.LightMode,
		},
		{
			name: "light without rpc is rejected",
			config: map[string]any{
				cmd.OptionNameNodeMode: "light",
			},
			wantErr: "light node requires blockchain-rpc-endpoint",
		},
		{
			name: "light with chequebook but no swap is rejected",
			config: map[string]any{
				cmd.OptionNameNodeMode:             "light",
				cmd.ConfigKeyBlockchainRpcEndpoint: testRPCEndpoint,
				cmd.OptionNameChequebookEnable:     true,
			},
			wantErr: "chequebook-enable requires swap-enable",
		},
		{
			name: "light with swap enables chequebook",
			config: map[string]any{
				cmd.OptionNameNodeMode:             "light",
				cmd.ConfigKeyBlockchainRpcEndpoint: testRPCEndpoint,
				cmd.OptionNameSwapEnable:           true,
			},
			wantMode: node.LightMode,
			wantOptions: map[string]bool{
				cmd.OptionNameSwapEnable:       true,
				cmd.OptionNameChequebookEnable: true,
			},
		},
		{
			name: "light rejects storage-incentives-enable",
			config: map[string]any{
				cmd.OptionNameNodeMode:                "light",
				cmd.ConfigKeyBlockchainRpcEndpoint:    testRPCEndpoint,
				cmd.OptionNameStorageIncentivesEnable: true,
			},
			wantErr: "light node cannot have storage-incentives-enable",
		},
		{
			name: "ultra-light succeeds",
			config: map[string]any{
				cmd.OptionNameNodeMode: "ultra-light",
			},
			wantMode: node.UltraLightMode,
		},
		{
			name: "ultra-light with rpc ignores rpc and succeeds",
			config: map[string]any{
				cmd.OptionNameNodeMode:             "ultra-light",
				cmd.ConfigKeyBlockchainRpcEndpoint: testRPCEndpoint,
			},
			wantMode: node.UltraLightMode,
		},
		{
			name: "ultra-light rejects swap-enable",
			config: map[string]any{
				cmd.OptionNameNodeMode:   "ultra-light",
				cmd.OptionNameSwapEnable: true,
			},
			wantErr: "ultra-light node cannot have swap-enable",
		},
		{
			name: "ultra-light rejects storage-incentives-enable",
			config: map[string]any{
				cmd.OptionNameNodeMode:                "ultra-light",
				cmd.OptionNameStorageIncentivesEnable: true,
			},
			wantErr: "ultra-light node cannot have storage-incentives-enable",
		},
		{
			name: "ultra-light rejects chequebook-enable",
			config: map[string]any{
				cmd.OptionNameNodeMode:         "ultra-light",
				cmd.OptionNameChequebookEnable: true,
			},
			wantErr: "ultra-light node cannot have chequebook-enable",
		},
		{
			name: "node-mode takes precedence over legacy full-node",
			config: map[string]any{
				cmd.OptionNameNodeMode:             "light",
				cmd.OptionNameFullNode:             true,
				cmd.ConfigKeyBlockchainRpcEndpoint: testRPCEndpoint,
			},
			wantMode: node.LightMode,
		},
		{
			name: "invalid node-mode value is rejected",
			config: map[string]any{
				cmd.OptionNameNodeMode: "superlight",
			},
			wantErr: "invalid node-mode",
		},
		{
			name: "uppercase node-mode is accepted",
			config: map[string]any{
				cmd.OptionNameNodeMode:             "FULL",
				cmd.ConfigKeyBlockchainRpcEndpoint: testRPCEndpoint,
			},
			wantMode: node.FullMode,
		},
		{
			name: "node-mode surrounded by whitespace is accepted",
			config: map[string]any{
				cmd.OptionNameNodeMode:             " light ",
				cmd.ConfigKeyBlockchainRpcEndpoint: testRPCEndpoint,
			},
			wantMode: node.LightMode,
		},
		{
			name: "whitespace-only node-mode is rejected",
			config: map[string]any{
				cmd.OptionNameNodeMode: "  ",
			},
			wantErr: "invalid node-mode",
		},
		{
			name: "full with whitespace-only rpc is rejected",
			config: map[string]any{
				cmd.OptionNameNodeMode:             "full",
				cmd.ConfigKeyBlockchainRpcEndpoint: "  ",
			},
			wantErr: "full node requires blockchain-rpc-endpoint",
		},
		{
			name: "light with whitespace-only rpc is rejected",
			config: map[string]any{
				cmd.OptionNameNodeMode:             "light",
				cmd.ConfigKeyBlockchainRpcEndpoint: "  ",
			},
			wantErr: "light node requires blockchain-rpc-endpoint",
		},
		{
			name: "light rejects reserve-capacity-doubling",
			config: map[string]any{
				cmd.OptionNameNodeMode:                "light",
				cmd.ConfigKeyBlockchainRpcEndpoint:    testRPCEndpoint,
				cmd.OptionNameReserveCapacityDoubling: 1,
			},
			wantErr: "reserve capacity doubling is only allowed for full nodes",
		},
		{
			name: "full accepts reserve-capacity-doubling",
			config: map[string]any{
				cmd.OptionNameNodeMode:                "full",
				cmd.ConfigKeyBlockchainRpcEndpoint:    testRPCEndpoint,
				cmd.OptionNameReserveCapacityDoubling: 1,
			},
			wantMode: node.FullMode,
		},
		{
			name: "light rejects chequebook-verification",
			config: map[string]any{
				cmd.OptionNameNodeMode:               "light",
				cmd.ConfigKeyBlockchainRpcEndpoint:   testRPCEndpoint,
				cmd.OptionNameSwapEnable:             true,
				cmd.OptionNameChequebookVerification: true,
			},
			wantErr: "chequebook-verification requires",
		},
		{
			name: "full with swap off rejects chequebook-verification",
			config: map[string]any{
				cmd.OptionNameNodeMode:               "full",
				cmd.ConfigKeyBlockchainRpcEndpoint:   testRPCEndpoint,
				cmd.OptionNameSwapEnable:             false,
				cmd.OptionNameChequebookVerification: true,
			},
			wantErr: "chequebook-verification requires",
		},
		{
			name: "full accepts chequebook-verification",
			config: map[string]any{
				cmd.OptionNameNodeMode:               "full",
				cmd.ConfigKeyBlockchainRpcEndpoint:   testRPCEndpoint,
				cmd.OptionNameChequebookVerification: true,
			},
			wantMode: node.FullMode,
		},
		{
			name: "light bootnode is rejected",
			config: map[string]any{
				cmd.OptionNameNodeMode:             "light",
				cmd.OptionNameBootnodeMode:         true,
				cmd.ConfigKeyBlockchainRpcEndpoint: testRPCEndpoint,
			},
			wantErr: "boot node must be started as a full node",
		},

		// ── node-mode unset: legacy behavior, as before node-mode ───────────────
		{
			// The most common pre-node-mode light config. chequebook-enable used
			// to default to true, so this node issued cheques; it must keep doing so.
			name: "legacy light with swap only keeps chequebook default",
			config: map[string]any{
				cmd.ConfigKeyBlockchainRpcEndpoint: testRPCEndpoint,
				cmd.OptionNameSwapEnable:           true,
			},
			wantMode: node.LightMode,
			wantOptions: map[string]bool{
				cmd.OptionNameSwapEnable:              true,
				cmd.OptionNameChequebookEnable:        true,
				cmd.OptionNameStorageIncentivesEnable: true,
			},
		},
		{
			// The old shipped default; chequebook stays gated on swap in NewBee.
			name: "legacy chequebook without swap starts",
			config: map[string]any{
				cmd.OptionNameChequebookEnable: true,
			},
			wantMode: node.UltraLightMode,
		},
		{
			name: "legacy full-node with rpc only keeps old defaults and leaves swap off",
			config: map[string]any{
				cmd.OptionNameFullNode:             true,
				cmd.ConfigKeyBlockchainRpcEndpoint: testRPCEndpoint,
			},
			wantMode: node.FullMode,
			wantOptions: map[string]bool{
				cmd.OptionNameSwapEnable:              false,
				cmd.OptionNameChequebookEnable:        true,
				cmd.OptionNameStorageIncentivesEnable: true,
			},
		},
		{
			name: "legacy full-node with all options set maps to full",
			config: map[string]any{
				cmd.OptionNameFullNode:                true,
				cmd.ConfigKeyBlockchainRpcEndpoint:    testRPCEndpoint,
				cmd.OptionNameSwapEnable:              true,
				cmd.OptionNameChequebookEnable:        true,
				cmd.OptionNameStorageIncentivesEnable: true,
			},
			wantMode: node.FullMode,
		},
		{
			name: "legacy full-node keeps explicit opt-outs",
			config: map[string]any{
				cmd.OptionNameFullNode:                true,
				cmd.ConfigKeyBlockchainRpcEndpoint:    testRPCEndpoint,
				cmd.OptionNameSwapEnable:              false,
				cmd.OptionNameChequebookEnable:        false,
				cmd.OptionNameStorageIncentivesEnable: false,
			},
			wantMode: node.FullMode,
			wantOptions: map[string]bool{
				cmd.OptionNameSwapEnable:              false,
				cmd.OptionNameChequebookEnable:        false,
				cmd.OptionNameStorageIncentivesEnable: false,
			},
		},
		{
			name: "legacy bootnode with storage-incentives false starts",
			config: map[string]any{
				cmd.OptionNameFullNode:                true,
				cmd.OptionNameBootnodeMode:            true,
				cmd.ConfigKeyBlockchainRpcEndpoint:    testRPCEndpoint,
				cmd.OptionNameStorageIncentivesEnable: false,
			},
			wantMode: node.FullMode,
		},
		{
			// Previous releases enabled the chain backend for every full node and
			// failed at chain init without an endpoint; keep failing, earlier.
			name: "legacy full-node without rpc is rejected",
			config: map[string]any{
				cmd.OptionNameFullNode: true,
			},
			wantErr: "full node requires blockchain-rpc-endpoint",
		},
		{
			name: "legacy with rpc infers light",
			config: map[string]any{
				cmd.ConfigKeyBlockchainRpcEndpoint: testRPCEndpoint,
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
			// base profile on a node without rpc. Releases before node-mode failed
			// it in NewBee; it is now rejected before the node is built.
			name: "legacy without rpc but with swap is rejected",
			config: map[string]any{
				cmd.OptionNameSwapEnable: true,
			},
			wantErr: "swap is enabled but the chain backend is not",
		},
		{
			name: "legacy light bootnode is rejected",
			config: map[string]any{
				cmd.OptionNameBootnodeMode:         true,
				cmd.ConfigKeyBlockchainRpcEndpoint: testRPCEndpoint,
			},
			wantErr: "boot node must be started as a full node",
		},
		{
			name: "legacy light rejects reserve-capacity-doubling",
			config: map[string]any{
				cmd.ConfigKeyBlockchainRpcEndpoint:    testRPCEndpoint,
				cmd.OptionNameReserveCapacityDoubling: 1,
			},
			wantErr: "reserve capacity doubling is only allowed for full nodes",
		},
		{
			// Before node-mode, a whitespace-only endpoint selected light mode
			// and failed at chain init; inference keeps that.
			name: "legacy whitespace-only rpc infers light",
			config: map[string]any{
				cmd.ConfigKeyBlockchainRpcEndpoint: " ",
			},
			wantMode: node.LightMode,
		},
		{
			name: "legacy full-node with whitespace-only rpc is rejected",
			config: map[string]any{
				cmd.OptionNameFullNode:             true,
				cmd.ConfigKeyBlockchainRpcEndpoint: " ",
			},
			wantErr: "full node requires blockchain-rpc-endpoint",
		},
		{
			// Legacy must not restore a default the operator overrode.
			name: "legacy explicit false is preserved",
			config: map[string]any{
				cmd.ConfigKeyBlockchainRpcEndpoint:    testRPCEndpoint,
				cmd.OptionNameSwapEnable:              true,
				cmd.OptionNameChequebookEnable:        false,
				cmd.OptionNameStorageIncentivesEnable: false,
			},
			wantMode: node.LightMode,
			wantOptions: map[string]bool{
				cmd.OptionNameChequebookEnable:        false,
				cmd.OptionNameStorageIncentivesEnable: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Start from the real start flags, so options a case leaves out
			// keep their production defaults.
			cfg := boundStartConfig(t, nil)
			for k, v := range tt.config {
				cfg.Set(k, v)
			}

			gotMode, err := cmd.ResolveNodeModeForTest(cfg)

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
				if got := cfg.GetBool(key); got != want {
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
		wantErr     string
		wantOptions map[string]bool
	}{
		{
			name:     "no flags selects legacy regime and keeps old defaults",
			args:     nil,
			wantMode: node.UltraLightMode,
			wantOptions: map[string]bool{
				cmd.OptionNameChequebookEnable:        true,
				cmd.OptionNameStorageIncentivesEnable: true,
			},
		},
		{
			name:     "legacy full-node flag with rpc",
			args:     []string{"--full-node", "--blockchain-rpc-endpoint=" + testRPCEndpoint},
			wantMode: node.FullMode,
			wantOptions: map[string]bool{
				cmd.OptionNameSwapEnable:              false,
				cmd.OptionNameChequebookEnable:        true,
				cmd.OptionNameStorageIncentivesEnable: true,
			},
		},
		{
			name:     "node-mode full with rpc implies the full stack",
			args:     []string{"--node-mode=full", "--blockchain-rpc-endpoint=" + testRPCEndpoint},
			wantMode: node.FullMode,
			wantOptions: map[string]bool{
				cmd.OptionNameSwapEnable:              true,
				cmd.OptionNameChequebookEnable:        true,
				cmd.OptionNameStorageIncentivesEnable: true,
			},
		},
		{
			name:     "node-mode light with rpc leaves swap and chequebook off",
			args:     []string{"--node-mode=light", "--blockchain-rpc-endpoint=" + testRPCEndpoint},
			wantMode: node.LightMode,
			wantOptions: map[string]bool{
				cmd.OptionNameSwapEnable:              false,
				cmd.OptionNameChequebookEnable:        false,
				cmd.OptionNameStorageIncentivesEnable: false,
			},
		},
		{
			name:     "node-mode full bootnode does not inherit chequebook or incentives defaults",
			args:     []string{"--node-mode=full", "--bootnode-mode", "--blockchain-rpc-endpoint=" + testRPCEndpoint},
			wantMode: node.FullMode,
			wantOptions: map[string]bool{
				cmd.OptionNameSwapEnable:              false,
				cmd.OptionNameChequebookEnable:        false,
				cmd.OptionNameStorageIncentivesEnable: false,
			},
		},
		{
			name:     "node-mode full bootnode with swap enables chequebook",
			args:     []string{"--node-mode=full", "--bootnode-mode", "--swap-enable", "--blockchain-rpc-endpoint=" + testRPCEndpoint},
			wantMode: node.FullMode,
			wantOptions: map[string]bool{
				cmd.OptionNameSwapEnable:              true,
				cmd.OptionNameChequebookEnable:        true,
				cmd.OptionNameStorageIncentivesEnable: false,
			},
		},
		{
			name:     "node-mode ultra-light leaves chequebook and incentives off",
			args:     []string{"--node-mode=ultra-light"},
			wantMode: node.UltraLightMode,
			wantOptions: map[string]bool{
				cmd.OptionNameSwapEnable:              false,
				cmd.OptionNameChequebookEnable:        false,
				cmd.OptionNameStorageIncentivesEnable: false,
			},
		},
		{
			name:    "node-mode ultra-light rejects chequebook-enable",
			args:    []string{"--node-mode=ultra-light", "--chequebook-enable"},
			wantErr: "ultra-light node cannot have chequebook-enable",
		},
		{
			name:     "node-mode full with incentives explicitly disabled",
			args:     []string{"--node-mode=full", "--blockchain-rpc-endpoint=" + testRPCEndpoint, "--storage-incentives-enable=false"},
			wantMode: node.FullMode,
			wantOptions: map[string]bool{
				cmd.OptionNameSwapEnable:              true,
				cmd.OptionNameChequebookEnable:        true,
				cmd.OptionNameStorageIncentivesEnable: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := boundStartConfig(t, tt.args)

			gotMode, err := cmd.ResolveNodeModeForTest(cfg)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
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
				if got := cfg.GetBool(key); got != want {
					t.Errorf("option %q: got %t, want %t", key, got, want)
				}
			}
		})
	}
}

func TestResolveNodeModeWarnings(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		args []string
		// wantErr is the expected resolution error, empty for none. want lists
		// substrings the log must contain; absent lists substrings it must not.
		wantErr      string
		want, absent []string
	}{
		{
			name: "ultra-light with rpc",
			args: []string{"--node-mode=ultra-light", "--blockchain-rpc-endpoint=" + testRPCEndpoint},
			want: []string{"blockchain-rpc-endpoint is ignored in ultra-light mode"},
		},
		{
			name:   "ultra-light without rpc",
			args:   []string{"--node-mode=ultra-light"},
			absent: []string{"blockchain-rpc-endpoint is ignored"},
		},
		{
			name:   "light with rpc",
			args:   []string{"--node-mode=light", "--blockchain-rpc-endpoint=" + testRPCEndpoint},
			absent: []string{"blockchain-rpc-endpoint is ignored"},
		},
		{
			name: "light without swap explains the upload limit",
			args: []string{"--node-mode=light", "--blockchain-rpc-endpoint=" + testRPCEndpoint},
			want: []string{"uploads and downloads are limited to the free pseudosettle allowance"},
		},
		{
			name:   "light with swap does not explain the upload limit",
			args:   []string{"--node-mode=light", "--swap-enable", "--blockchain-rpc-endpoint=" + testRPCEndpoint},
			absent: []string{"pseudosettle allowance"},
		},
		{
			name:   "full without swap does not explain the upload limit",
			args:   []string{"--node-mode=full", "--swap-enable=false", "--blockchain-rpc-endpoint=" + testRPCEndpoint},
			absent: []string{"pseudosettle allowance"},
		},
		{
			name:   "legacy light does not explain the upload limit",
			args:   []string{"--blockchain-rpc-endpoint=" + testRPCEndpoint},
			absent: []string{"pseudosettle allowance"},
		},
		{
			name: "full bootnode with storage incentives enabled",
			args: []string{"--node-mode=full", "--bootnode-mode", "--storage-incentives-enable", "--blockchain-rpc-endpoint=" + testRPCEndpoint},
			want: []string{"storage-incentives-enable has no effect on a bootnode"},
		},
		{
			name:   "full bootnode without storage incentives",
			args:   []string{"--node-mode=full", "--bootnode-mode", "--blockchain-rpc-endpoint=" + testRPCEndpoint},
			absent: []string{"has no effect on a bootnode"},
		},
		{
			name: "legacy full node lists the migration",
			args: []string{"--full-node", "--blockchain-rpc-endpoint=" + testRPCEndpoint},
			want: []string{"node-mode is not set", `"set"=["node-mode=full","swap-enable=false"]`, `"remove"=["full-node"]`},
		},
		{
			name:   "legacy light node lists options to remove",
			args:   []string{"--blockchain-rpc-endpoint=" + testRPCEndpoint, "--storage-incentives-enable"},
			want:   []string{`"set"=["node-mode=light"]`, `"remove"=["storage-incentives-enable"]`},
			absent: []string{"behavior_change"},
		},
		{
			name: "legacy full node with verification notes the swap change",
			args: []string{"--full-node", "--chequebook-verification", "--blockchain-rpc-endpoint=" + testRPCEndpoint},
			want: []string{`"set"=["node-mode=full"]`, "set swap-enable=false and remove chequebook-verification"},
		},
		{
			name: "legacy whitespace-only rpc notes that light needs an endpoint",
			args: []string{"--blockchain-rpc-endpoint= "},
			want: []string{`"set"=["node-mode=light"]`, "blockchain-rpc-endpoint contains only whitespace"},
		},
		{
			name:    "legacy bootnode error is not preceded by the migration warning",
			args:    []string{"--bootnode-mode", "--blockchain-rpc-endpoint=" + testRPCEndpoint},
			wantErr: "boot node must be started as a full node",
			absent:  []string{"node-mode is not set"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := boundStartConfig(t, tc.args)

			var buf bytes.Buffer
			logger := log.NewLogger("resolve-node-mode-"+t.Name(), log.WithSink(&buf))
			_, err := cmd.ResolveNodeModeWithLoggerForTest(cfg, logger)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
			for _, w := range tc.want {
				if !strings.Contains(buf.String(), w) {
					t.Errorf("log does not contain %q; log: %s", w, buf.String())
				}
			}
			for _, a := range tc.absent {
				if strings.Contains(buf.String(), a) {
					t.Errorf("log contains %q; log: %s", a, buf.String())
				}
			}
		})
	}
}

// TestLegacyMigrationPreservesBehavior applies the migration listed by the
// deprecation warning to every legacy combination of the mode-related options
// and checks that the node then runs the same services with node-mode set.
// Where behavior cannot be preserved, the warning must say so. Combinations
// rejected before migration must be exactly those releases before node-mode
// rejected.
//
// The chequebook is modeled as running, which needs swap. The API's
// chequebookEnabled flag reports the option itself and does change for nodes
// without swap: chequebook-enable follows swap-enable once node-mode is set.
func TestLegacyMigrationPreservesBehavior(t *testing.T) {
	t.Parallel()

	tri := []string{"", "true", "false"}
	type effect struct {
		mode                                       node.NodeMode
		swap, chequebook, incentives, verification bool
	}
	effectOf := func(cfg *viper.Viper, mode node.NodeMode, bootnode bool) effect {
		swap := cfg.GetBool(cmd.OptionNameSwapEnable)
		return effect{
			mode:         mode,
			swap:         swap,
			chequebook:   swap && cfg.GetBool(cmd.OptionNameChequebookEnable),
			incentives:   mode == node.FullMode && !bootnode && cfg.GetBool(cmd.OptionNameStorageIncentivesEnable),
			verification: cfg.GetBool(cmd.OptionNameChequebookVerification),
		}
	}

	var checked, rejected, changed int
	for _, bootnode := range []bool{false, true} {
		for _, rpc := range []bool{false, true} {
			for _, fullNode := range tri {
				for _, swap := range tri {
					for _, chequebook := range tri {
						for _, incentives := range tri {
							for _, verification := range []string{"", "true"} {
								var args []string
								add := func(name, value string) {
									if value != "" {
										args = append(args, "--"+name+"="+value)
									}
								}
								if bootnode {
									add(cmd.OptionNameBootnodeMode, "true")
								}
								if rpc {
									add("blockchain-rpc-endpoint", testRPCEndpoint)
								}
								add(cmd.OptionNameFullNode, fullNode)
								add(cmd.OptionNameSwapEnable, swap)
								add(cmd.OptionNameChequebookEnable, chequebook)
								add(cmd.OptionNameStorageIncentivesEnable, incentives)
								add(cmd.OptionNameChequebookVerification, verification)

								// Rules of releases before node-mode: a bootnode must be
								// full, a full node needs a chain, swap needs a chain,
								// and verification needs a full node with chequebook and
								// chain. chequebook-enable defaults to true.
								full := fullNode == "true"
								chain := full || rpc
								wantRejected := (bootnode && !full) || (full && !rpc) || (swap == "true" && !chain) ||
									(verification == "true" && (!full || chequebook == "false" || !chain))

								legacyCfg := boundStartConfig(t, args)
								mode, set, remove, notes, err := cmd.LegacyMigrationForTest(legacyCfg)
								if (err != nil) != wantRejected {
									t.Errorf("%v: rejected=%t, want %t (err: %v)", args, err != nil, wantRejected, err)
									continue
								}
								if err != nil {
									rejected++
									continue
								}
								checked++
								before := effectOf(legacyCfg, mode, bootnode)

								var migrated []string
							next:
								for _, a := range args {
									for _, r := range remove {
										if strings.HasPrefix(a, "--"+r+"=") {
											continue next
										}
									}
									migrated = append(migrated, a)
								}
								for _, kv := range set {
									k, v, _ := strings.Cut(kv, "=")
									migrated = append(migrated, "--"+k+"="+v)
								}

								migratedCfg := boundStartConfig(t, migrated)
								gotMode, err := cmd.ResolveNodeModeForTest(migratedCfg)
								if len(notes) > 0 {
									changed++
									if err == nil && effectOf(migratedCfg, gotMode, bootnode) == before {
										t.Errorf("%v: warning notes a behavior change %v, but the migrated config %v behaves the same", args, notes, migrated)
									}
									continue
								}
								if err != nil {
									t.Errorf("%v: migrated config %v fails: %v", args, migrated, err)
									continue
								}
								if after := effectOf(migratedCfg, gotMode, bootnode); after != before {
									t.Errorf("%v: migrated config %v changes behavior: before %+v, after %+v", args, migrated, before, after)
								}
							}
						}
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no legacy configuration was checked")
	}
	t.Logf("checked %d legacy configurations (%d with a noted behavior change), %d rejected as before node-mode", checked, changed, rejected)
}

func TestResolveNodeModeTrimsRPCEndpoint(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "node-mode set trims the endpoint", args: []string{"--node-mode=full", "--blockchain-rpc-endpoint= " + testRPCEndpoint + " "}, want: testRPCEndpoint},
		{name: "legacy keeps the endpoint as configured", args: []string{"--full-node", "--blockchain-rpc-endpoint= " + testRPCEndpoint + " "}, want: " " + testRPCEndpoint + " "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := boundStartConfig(t, tc.args)
			if _, err := cmd.ResolveNodeModeForTest(cfg); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := cfg.GetString(cmd.ConfigKeyBlockchainRpcEndpoint); got != tc.want {
				t.Errorf("endpoint: got %q, want %q", got, tc.want)
			}
		})
	}
}

// boundStartConfig returns a viper bound to the real start command flags
// parsed from args, as production does.
func boundStartConfig(t *testing.T, args []string) *viper.Viper {
	t.Helper()

	root, err := cmd.NewCommand(cmd.WithHomeDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	startCmd := root.SubCommandForTest("start")
	if startCmd == nil {
		t.Fatal("start subcommand not found")
	}
	if err := startCmd.ParseFlags(args); err != nil {
		t.Fatal(err)
	}
	cfg := viper.New()
	if err := cmd.BindStartFlagsForTest(cfg, startCmd); err != nil {
		t.Fatal(err)
	}
	return cfg
}
