// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"github.com/ethersphere/bee/pkg/swarm"
	yaml "gopkg.in/yaml.v2"

	"github.com/spf13/cobra"
	"path/filepath"
)

func (c *command) initConfigurateOptionsCmd() (err error) {

	const (
		optionNameDataDir              = "data-dir"
		optionNameDBCapacity           = "db-capacity"
		optionNamePassword             = "password"
		optionNamePasswordFile         = "password-file"
		optionNameAPIAddr              = "api-addr"
		optionNameP2PAddr              = "p2p-addr"
		optionNameNATAddr              = "nat-addr"
		optionNameP2PWSEnable          = "p2p-ws-enable"
		optionNameP2PQUICEnable        = "p2p-quic-enable"
		optionNameDebugAPIEnable       = "debug-api-enable"
		optionNameDebugAPIAddr         = "debug-api-addr"
		optionNameBootnodes            = "bootnode"
		optionNameNetworkID            = "network-id"
		optionWelcomeMessage           = "welcome-message"
		optionCORSAllowedOrigins       = "cors-allowed-origins"
		optionNameTracingEnabled       = "tracing-enable"
		optionNameTracingEndpoint      = "tracing-endpoint"
		optionNameTracingServiceName   = "tracing-service-name"
		optionNameVerbosity            = "verbosity"
		optionNameGlobalPinningEnabled = "global-pinning-enable"
		optionNamePaymentThreshold     = "payment-threshold"
		optionNamePaymentTolerance     = "payment-tolerance"
	)

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Print configuration options",
		RunE: func(cmd *cobra.Command, args []string) (err error) {

			if len(args) > 0 {
				return cmd.Help()
			}

			d := c.config.AllSettings()
			bs, err := yaml.Marshal(d)
			if err != nil {
				cmd.Printf("unable to marshal config to yaml: %v", err)
			}
			cmd.Printf("%+v\n", string(bs))
			return nil

		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return c.config.BindPFlags(cmd.Flags())
		},
	}

	cmd.Flags().String(optionNameDataDir, filepath.Join(c.homeDir, ".bee"), "data directory")
	cmd.Flags().Uint64(optionNameDBCapacity, 5000000, fmt.Sprintf("db capacity in chunks, multiply by %d to get approximate capacity in bytes", swarm.ChunkSize))
	cmd.Flags().String(optionNamePassword, "", "password for decrypting keys")
	cmd.Flags().String(optionNamePasswordFile, "", "path to a file that contains password for decrypting keys")
	cmd.Flags().String(optionNameAPIAddr, ":8080", "HTTP API listen address")
	cmd.Flags().String(optionNameP2PAddr, ":7070", "P2P listen address")
	cmd.Flags().String(optionNameNATAddr, "", "NAT exposed address")
	cmd.Flags().Bool(optionNameP2PWSEnable, false, "enable P2P WebSocket transport")
	cmd.Flags().Bool(optionNameP2PQUICEnable, false, "enable P2P QUIC transport")
	cmd.Flags().StringSlice(optionNameBootnodes, []string{"/dnsaddr/bootnode.ethswarm.org"}, "initial nodes to connect to")
	cmd.Flags().Bool(optionNameDebugAPIEnable, false, "enable debug HTTP API")
	cmd.Flags().String(optionNameDebugAPIAddr, ":6060", "debug HTTP API listen address")
	cmd.Flags().Uint64(optionNameNetworkID, 1, "ID of the Swarm network")
	cmd.Flags().StringSlice(optionCORSAllowedOrigins, []string{}, "origins with CORS headers enabled")
	cmd.Flags().Bool(optionNameTracingEnabled, false, "enable tracing")
	cmd.Flags().String(optionNameTracingEndpoint, "127.0.0.1:6831", "endpoint to send tracing data")
	cmd.Flags().String(optionNameTracingServiceName, "bee", "service name identifier for tracing")
	cmd.Flags().String(optionNameVerbosity, "info", "log verbosity level 0=silent, 1=error, 2=warn, 3=info, 4=debug, 5=trace")
	cmd.Flags().String(optionWelcomeMessage, "", "send a welcome message string during handshakes")
	cmd.Flags().Uint64(optionNamePaymentThreshold, 100000, "threshold in BZZ where you expect to get paid from your peers")
	cmd.Flags().Uint64(optionNamePaymentTolerance, 10000, "excess debt above payment threshold in BZZ where you disconnect from your peer")

	c.root.AddCommand(cmd)

	return nil

}
