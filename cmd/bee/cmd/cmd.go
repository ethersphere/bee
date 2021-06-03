// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	optionNameDataDir                    = "data-dir"
	optionNameCacheCapacity              = "cache-capacity"
	optionNameDBOpenFilesLimit           = "db-open-files-limit"
	optionNameDBBlockCacheCapacity       = "db-block-cache-capacity"
	optionNameDBWriteBufferSize          = "db-write-buffer-size"
	optionNameDBDisableSeeksCompaction   = "db-disable-seeks-compaction"
	optionNamePassword                   = "password"
	optionNamePasswordFile               = "password-file"
	optionNameAPIAddr                    = "api-addr"
	optionNameP2PAddr                    = "p2p-addr"
	optionNameNATAddr                    = "nat-addr"
	optionNameP2PWSEnable                = "p2p-ws-enable"
	optionNameP2PQUICEnable              = "p2p-quic-enable"
	optionNameDebugAPIEnable             = "debug-api-enable"
	optionNameDebugAPIAddr               = "debug-api-addr"
	optionNameBootnodes                  = "bootnode"
	optionNameNetworkID                  = "network-id"
	optionWelcomeMessage                 = "welcome-message"
	optionCORSAllowedOrigins             = "cors-allowed-origins"
	optionNameStandalone                 = "standalone"
	optionNameTracingEnabled             = "tracing-enable"
	optionNameTracingEndpoint            = "tracing-endpoint"
	optionNameTracingServiceName         = "tracing-service-name"
	optionNameVerbosity                  = "verbosity"
	optionNameGlobalPinningEnabled       = "global-pinning-enable"
	optionNamePaymentThreshold           = "payment-threshold"
	optionNamePaymentTolerance           = "payment-tolerance"
	optionNamePaymentEarly               = "payment-early"
	optionNameResolverEndpoints          = "resolver-options"
	optionNameBootnodeMode               = "bootnode-mode"
	optionNameGatewayMode                = "gateway-mode"
	optionNameClefSignerEnable           = "clef-signer-enable"
	optionNameClefSignerEndpoint         = "clef-signer-endpoint"
	optionNameClefSignerEthereumAddress  = "clef-signer-ethereum-address"
	optionNameSwapEndpoint               = "swap-endpoint"
	optionNameSwapFactoryAddress         = "swap-factory-address"
	optionNameSwapLegacyFactoryAddresses = "swap-legacy-factory-addresses"
	optionNameSwapInitialDeposit         = "swap-initial-deposit"
	optionNameSwapEnable                 = "swap-enable"
	optionNameTransactionHash            = "transaction"
	optionNameSwapDeploymentGasPrice     = "swap-deployment-gas-price"
	optionNameFullNode                   = "full-node"
	optionNamePostageContractAddress     = "postage-stamp-address"
	optionNamePriceOracleAddress         = "price-oracle-address"
	optionNameBlockTime                  = "block-time"
)

func init() {
	cobra.EnableCommandSorting = false
}

type command struct {
	root           *cobra.Command
	config         *viper.Viper
	passwordReader passwordReader
	cfgFile        string
	homeDir        string
}

type option func(*command)

func newCommand(opts ...option) (c *command, err error) {
	c = &command{
		root: &cobra.Command{
			Use:           "bee",
			Short:         "Ethereum Swarm Bee",
			SilenceErrors: true,
			SilenceUsage:  true,
			PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
				return c.initConfig()
			},
		},
	}

	for _, o := range opts {
		o(c)
	}
	if c.passwordReader == nil {
		c.passwordReader = new(stdInPasswordReader)
	}

	// Find home directory.
	if err := c.setHomeDir(); err != nil {
		return nil, err
	}

	c.initGlobalFlags()

	if err := c.initStartCmd(); err != nil {
		return nil, err
	}

	if err := c.initInitCmd(); err != nil {
		return nil, err
	}

	if err := c.initDeployCmd(); err != nil {
		return nil, err
	}

	c.initVersionCmd()
	c.initDBCmd()

	if err := c.initConfigurateOptionsCmd(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *command) Execute() (err error) {
	return c.root.Execute()
}

// Execute parses command line arguments and runs appropriate functions.
func Execute() (err error) {
	c, err := newCommand()
	if err != nil {
		return err
	}
	return c.Execute()
}

func (c *command) initGlobalFlags() {
	globalFlags := c.root.PersistentFlags()
	globalFlags.StringVar(&c.cfgFile, "config", "", "config file (default is $HOME/.bee.yaml)")
}

func (c *command) initConfig() (err error) {
	config := viper.New()
	configName := ".bee"
	if c.cfgFile != "" {
		// Use config file from the flag.
		config.SetConfigFile(c.cfgFile)
	} else {
		// Search config in home directory with name ".bee" (without extension).
		config.AddConfigPath(c.homeDir)
		config.SetConfigName(configName)
	}

	// Environment
	config.SetEnvPrefix("bee")
	config.AutomaticEnv() // read in environment variables that match
	config.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	if c.homeDir != "" && c.cfgFile == "" {
		c.cfgFile = filepath.Join(c.homeDir, configName+".yaml")
	}

	// If a config file is found, read it in.
	if err := config.ReadInConfig(); err != nil {
		var e viper.ConfigFileNotFoundError
		if !errors.As(err, &e) {
			return err
		}
	}
	c.config = config
	return nil
}

func (c *command) setHomeDir() (err error) {
	if c.homeDir != "" {
		return
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	c.homeDir = dir
	return nil
}

func (c *command) setAllFlags(cmd *cobra.Command) {
	cmd.Flags().String(optionNameDataDir, filepath.Join(c.homeDir, ".bee"), "data directory")
	cmd.Flags().Uint64(optionNameCacheCapacity, 1000000, fmt.Sprintf("cache capacity in chunks, multiply by %d to get approximate capacity in bytes", swarm.ChunkSize))
	cmd.Flags().Uint64(optionNameDBOpenFilesLimit, 200, "number of open files allowed by database")
	cmd.Flags().Uint64(optionNameDBBlockCacheCapacity, 32*1024*1024, "size of block cache of the database in bytes")
	cmd.Flags().Uint64(optionNameDBWriteBufferSize, 32*1024*1024, "size of the database write buffer in bytes")
	cmd.Flags().Bool(optionNameDBDisableSeeksCompaction, false, "disables db compactions triggered by seeks")
	cmd.Flags().String(optionNamePassword, "", "password for decrypting keys")
	cmd.Flags().String(optionNamePasswordFile, "", "path to a file that contains password for decrypting keys")
	cmd.Flags().String(optionNameAPIAddr, ":1633", "HTTP API listen address")
	cmd.Flags().String(optionNameP2PAddr, ":1634", "P2P listen address")
	cmd.Flags().String(optionNameNATAddr, "", "NAT exposed address")
	cmd.Flags().Bool(optionNameP2PWSEnable, false, "enable P2P WebSocket transport")
	cmd.Flags().Bool(optionNameP2PQUICEnable, false, "enable P2P QUIC transport")
	cmd.Flags().StringSlice(optionNameBootnodes, []string{"/dnsaddr/bootnode.ethswarm.org"}, "initial nodes to connect to")
	cmd.Flags().Bool(optionNameDebugAPIEnable, false, "enable debug HTTP API")
	cmd.Flags().String(optionNameDebugAPIAddr, ":1635", "debug HTTP API listen address")
	cmd.Flags().Uint64(optionNameNetworkID, 1, "ID of the Swarm network")
	cmd.Flags().StringSlice(optionCORSAllowedOrigins, []string{}, "origins with CORS headers enabled")
	cmd.Flags().Bool(optionNameStandalone, false, "whether we want the node to start with no listen addresses for p2p")
	cmd.Flags().Bool(optionNameTracingEnabled, false, "enable tracing")
	cmd.Flags().String(optionNameTracingEndpoint, "127.0.0.1:6831", "endpoint to send tracing data")
	cmd.Flags().String(optionNameTracingServiceName, "bee", "service name identifier for tracing")
	cmd.Flags().String(optionNameVerbosity, "info", "log verbosity level 0=silent, 1=error, 2=warn, 3=info, 4=debug, 5=trace")
	cmd.Flags().String(optionWelcomeMessage, "", "send a welcome message string during handshakes")
	cmd.Flags().Bool(optionNameGlobalPinningEnabled, false, "enable global pinning")
	cmd.Flags().String(optionNamePaymentThreshold, "100000000", "threshold in BZZ where you expect to get paid from your peers")
	cmd.Flags().String(optionNamePaymentTolerance, "100000000", "excess debt above payment threshold in BZZ where you disconnect from your peer")
	cmd.Flags().String(optionNamePaymentEarly, "10000000", "amount in BZZ below the peers payment threshold when we initiate settlement")
	cmd.Flags().StringSlice(optionNameResolverEndpoints, []string{}, "ENS compatible API endpoint for a TLD and with contract address, can be repeated, format [tld:][contract-addr@]url")
	cmd.Flags().Bool(optionNameGatewayMode, false, "disable a set of sensitive features in the api")
	cmd.Flags().Bool(optionNameBootnodeMode, false, "cause the node to always accept incoming connections")
	cmd.Flags().Bool(optionNameClefSignerEnable, false, "enable clef signer")
	cmd.Flags().String(optionNameClefSignerEndpoint, "", "clef signer endpoint")
	cmd.Flags().String(optionNameClefSignerEthereumAddress, "", "ethereum address to use from clef signer")
	cmd.Flags().String(optionNameSwapEndpoint, "ws://localhost:8546", "swap ethereum blockchain endpoint")
	cmd.Flags().String(optionNameSwapFactoryAddress, "", "swap factory addresses")
	cmd.Flags().StringSlice(optionNameSwapLegacyFactoryAddresses, nil, "legacy swap factory addresses")
	cmd.Flags().String(optionNameSwapInitialDeposit, "10000000000000000", "initial deposit if deploying a new chequebook")
	cmd.Flags().Bool(optionNameSwapEnable, true, "enable swap")
	cmd.Flags().Bool(optionNameFullNode, false, "cause the node to start in full mode")
	cmd.Flags().String(optionNamePostageContractAddress, "", "postage stamp contract address")
	cmd.Flags().String(optionNamePriceOracleAddress, "", "price oracle contract address")
	cmd.Flags().String(optionNameTransactionHash, "", "proof-of-identity transaction hash")
	cmd.Flags().Uint64(optionNameBlockTime, 15, "chain block time")
	cmd.Flags().String(optionNameSwapDeploymentGasPrice, "", "gas price in wei to use for deployment and funding")
}

func newLogger(cmd *cobra.Command, verbosity string) (logging.Logger, error) {
	var logger logging.Logger
	switch verbosity {
	case "0", "silent":
		logger = logging.New(ioutil.Discard, 0)
	case "1", "error":
		logger = logging.New(cmd.OutOrStdout(), logrus.ErrorLevel)
	case "2", "warn":
		logger = logging.New(cmd.OutOrStdout(), logrus.WarnLevel)
	case "3", "info":
		logger = logging.New(cmd.OutOrStdout(), logrus.InfoLevel)
	case "4", "debug":
		logger = logging.New(cmd.OutOrStdout(), logrus.DebugLevel)
	case "5", "trace":
		logger = logging.New(cmd.OutOrStdout(), logrus.TraceLevel)
	default:
		return nil, fmt.Errorf("unknown verbosity level %q", verbosity)
	}
	return logger, nil
}
