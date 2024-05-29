// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	chaincfg "github.com/ethersphere/bee/v2/pkg/config"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/node"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	optionNameDataDir                      = "data-dir"
	optionNameCacheCapacity                = "cache-capacity"
	optionNameDBOpenFilesLimit             = "db-open-files-limit"
	optionNameDBBlockCacheCapacity         = "db-block-cache-capacity"
	optionNameDBWriteBufferSize            = "db-write-buffer-size"
	optionNameDBDisableSeeksCompaction     = "db-disable-seeks-compaction"
	optionNamePassword                     = "password"
	optionNamePasswordFile                 = "password-file"
	optionNameAPIAddr                      = "api-addr"
	optionNameP2PAddr                      = "p2p-addr"
	optionNameNATAddr                      = "nat-addr"
	optionNameP2PWSEnable                  = "p2p-ws-enable"
	optionNameBootnodes                    = "bootnode"
	optionNameNetworkID                    = "network-id"
	optionWelcomeMessage                   = "welcome-message"
	optionCORSAllowedOrigins               = "cors-allowed-origins"
	optionNameTracingEnabled               = "tracing-enable"
	optionNameTracingEndpoint              = "tracing-endpoint"
	optionNameTracingHost                  = "tracing-host"
	optionNameTracingPort                  = "tracing-port"
	optionNameTracingServiceName           = "tracing-service-name"
	optionNameVerbosity                    = "verbosity"
	optionNamePaymentThreshold             = "payment-threshold"
	optionNamePaymentTolerance             = "payment-tolerance-percent"
	optionNamePaymentEarly                 = "payment-early-percent"
	optionNameResolverEndpoints            = "resolver-options"
	optionNameBootnodeMode                 = "bootnode-mode"
	optionNameClefSignerEnable             = "clef-signer-enable"
	optionNameClefSignerEndpoint           = "clef-signer-endpoint"
	optionNameClefSignerEthereumAddress    = "clef-signer-ethereum-address"
	optionNameSwapEndpoint                 = "swap-endpoint" // deprecated: use rpc endpoint instead
	optionNameBlockchainRpcEndpoint        = "blockchain-rpc-endpoint"
	optionNameSwapFactoryAddress           = "swap-factory-address"
	optionNameSwapInitialDeposit           = "swap-initial-deposit"
	optionNameSwapEnable                   = "swap-enable"
	optionNameChequebookEnable             = "chequebook-enable"
	optionNameSwapDeploymentGasPrice       = "swap-deployment-gas-price"
	optionNameFullNode                     = "full-node"
	optionNamePostageContractAddress       = "postage-stamp-address"
	optionNamePostageContractStartBlock    = "postage-stamp-start-block"
	optionNamePriceOracleAddress           = "price-oracle-address"
	optionNameRedistributionAddress        = "redistribution-address"
	optionNameStakingAddress               = "staking-address"
	optionNameBlockTime                    = "block-time"
	optionWarmUpTime                       = "warmup-time"
	optionNameMainNet                      = "mainnet"
	optionNameRetrievalCaching             = "cache-retrieval"
	optionNameDevReserveCapacity           = "dev-reserve-capacity"
	optionNameResync                       = "resync"
	optionNamePProfBlock                   = "pprof-profile"
	optionNamePProfMutex                   = "pprof-mutex"
	optionNameStaticNodes                  = "static-nodes"
	optionNameAllowPrivateCIDRs            = "allow-private-cidrs"
	optionNameSleepAfter                   = "sleep-after"
	optionNameRestrictedAPI                = "restricted"
	optionNameTokenEncryptionKey           = "token-encryption-key"
	optionNameAdminPasswordHash            = "admin-password"
	optionNameUsePostageSnapshot           = "use-postage-snapshot"
	optionNameStorageIncentivesEnable      = "storage-incentives-enable"
	optionNameStateStoreCacheCapacity      = "statestore-cache-capacity"
	optionNameTargetNeighborhood           = "target-neighborhood"
	optionNameNeighborhoodSuggester        = "neighborhood-suggester"
	optionNameWhitelistedWithdrawalAddress = "withdrawal-addresses-whitelist"
)

// nolint:gochecknoinits
func init() {
	cobra.EnableCommandSorting = false
}

type command struct {
	root             *cobra.Command
	config           *viper.Viper
	passwordReader   passwordReader
	cfgFile          string
	homeDir          string
	isWindowsService bool
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

	if err := c.initCommandVariables(); err != nil {
		return nil, err
	}

	if err := c.initStartCmd(); err != nil {
		return nil, err
	}

	if err := c.initStartDevCmd(); err != nil {
		return nil, err
	}

	if err := c.initHasherCmd(); err != nil {
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
	if err := c.initSplitCmd(); err != nil {
		return nil, err
	}

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

func (c *command) initCommandVariables() error {
	isWindowsService, err := isWindowsService()
	if err != nil {
		return fmt.Errorf("failed to determine if we are running in service: %w", err)
	}

	c.isWindowsService = isWindowsService

	return nil
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
	cmd.Flags().Uint64(optionNameCacheCapacity, 1_000_000, fmt.Sprintf("cache capacity in chunks, multiply by %d to get approximate capacity in bytes", swarm.ChunkSize))
	cmd.Flags().Uint64(optionNameDBOpenFilesLimit, 200, "number of open files allowed by database")
	cmd.Flags().Uint64(optionNameDBBlockCacheCapacity, 32*1024*1024, "size of block cache of the database in bytes")
	cmd.Flags().Uint64(optionNameDBWriteBufferSize, 32*1024*1024, "size of the database write buffer in bytes")
	cmd.Flags().Bool(optionNameDBDisableSeeksCompaction, true, "disables db compactions triggered by seeks")
	cmd.Flags().String(optionNamePassword, "", "password for decrypting keys")
	cmd.Flags().String(optionNamePasswordFile, "", "path to a file that contains password for decrypting keys")
	cmd.Flags().String(optionNameAPIAddr, ":1633", "HTTP API listen address")
	cmd.Flags().String(optionNameP2PAddr, ":1634", "P2P listen address")
	cmd.Flags().String(optionNameNATAddr, "", "NAT exposed address")
	cmd.Flags().Bool(optionNameP2PWSEnable, false, "enable P2P WebSocket transport")
	cmd.Flags().StringSlice(optionNameBootnodes, []string{""}, "initial nodes to connect to")
	cmd.Flags().Uint64(optionNameNetworkID, chaincfg.Mainnet.NetworkID, "ID of the Swarm network")
	cmd.Flags().StringSlice(optionCORSAllowedOrigins, []string{}, "origins with CORS headers enabled")
	cmd.Flags().Bool(optionNameTracingEnabled, false, "enable tracing")
	cmd.Flags().String(optionNameTracingEndpoint, "127.0.0.1:6831", "endpoint to send tracing data")
	cmd.Flags().String(optionNameTracingHost, "", "host to send tracing data")
	cmd.Flags().String(optionNameTracingPort, "", "port to send tracing data")
	cmd.Flags().String(optionNameTracingServiceName, "bee", "service name identifier for tracing")
	cmd.Flags().String(optionNameVerbosity, "info", "log verbosity level 0=silent, 1=error, 2=warn, 3=info, 4=debug, 5=trace")
	cmd.Flags().String(optionWelcomeMessage, "", "send a welcome message string during handshakes")
	cmd.Flags().String(optionNamePaymentThreshold, "13500000", "threshold in BZZ where you expect to get paid from your peers")
	cmd.Flags().Int64(optionNamePaymentTolerance, 25, "excess debt above payment threshold in percentages where you disconnect from your peer")
	cmd.Flags().Int64(optionNamePaymentEarly, 50, "percentage below the peers payment threshold when we initiate settlement")
	cmd.Flags().StringSlice(optionNameResolverEndpoints, []string{}, "ENS compatible API endpoint for a TLD and with contract address, can be repeated, format [tld:][contract-addr@]url")
	cmd.Flags().Bool(optionNameBootnodeMode, false, "cause the node to always accept incoming connections")
	cmd.Flags().Bool(optionNameClefSignerEnable, false, "enable clef signer")
	cmd.Flags().String(optionNameClefSignerEndpoint, "", "clef signer endpoint")
	cmd.Flags().String(optionNameClefSignerEthereumAddress, "", "blockchain address to use from clef signer")
	cmd.Flags().String(optionNameSwapEndpoint, "", "swap blockchain endpoint") // deprecated: use rpc endpoint instead
	cmd.Flags().String(optionNameBlockchainRpcEndpoint, "", "rpc blockchain endpoint")
	cmd.Flags().String(optionNameSwapFactoryAddress, "", "swap factory addresses")
	cmd.Flags().String(optionNameSwapInitialDeposit, "0", "initial deposit if deploying a new chequebook")
	cmd.Flags().Bool(optionNameSwapEnable, false, "enable swap")
	cmd.Flags().Bool(optionNameChequebookEnable, true, "enable chequebook")
	cmd.Flags().Bool(optionNameFullNode, false, "cause the node to start in full mode")
	cmd.Flags().String(optionNamePostageContractAddress, "", "postage stamp contract address")
	cmd.Flags().Uint64(optionNamePostageContractStartBlock, 0, "postage stamp contract start block number")
	cmd.Flags().String(optionNamePriceOracleAddress, "", "price oracle contract address")
	cmd.Flags().String(optionNameRedistributionAddress, "", "redistribution contract address")
	cmd.Flags().String(optionNameStakingAddress, "", "staking contract address")
	cmd.Flags().Uint64(optionNameBlockTime, 15, "chain block time")
	cmd.Flags().String(optionNameSwapDeploymentGasPrice, "", "gas price in wei to use for deployment and funding")
	cmd.Flags().Duration(optionWarmUpTime, time.Minute*5, "time to warmup the node before some major protocols can be kicked off")
	cmd.Flags().Bool(optionNameMainNet, true, "triggers connect to main net bootnodes.")
	cmd.Flags().Bool(optionNameRetrievalCaching, true, "enable forwarded content caching")
	cmd.Flags().Bool(optionNameResync, false, "forces the node to resync postage contract data")
	cmd.Flags().Bool(optionNamePProfBlock, false, "enable pprof block profile")
	cmd.Flags().Bool(optionNamePProfMutex, false, "enable pprof mutex profile")
	cmd.Flags().StringSlice(optionNameStaticNodes, []string{}, "protect nodes from getting kicked out on bootnode")
	cmd.Flags().Bool(optionNameAllowPrivateCIDRs, false, "allow to advertise private CIDRs to the public network")
	cmd.Flags().Bool(optionNameRestrictedAPI, false, "enable permission check on the http APIs")
	cmd.Flags().String(optionNameTokenEncryptionKey, "", "admin username to get the security token")
	cmd.Flags().String(optionNameAdminPasswordHash, "", "bcrypt hash of the admin password to get the security token")
	cmd.Flags().Bool(optionNameUsePostageSnapshot, false, "bootstrap node using postage snapshot from the network")
	cmd.Flags().Bool(optionNameStorageIncentivesEnable, true, "enable storage incentives feature")
	cmd.Flags().Uint64(optionNameStateStoreCacheCapacity, 100_000, "lru memory caching capacity in number of statestore entries")
	cmd.Flags().String(optionNameTargetNeighborhood, "", "neighborhood to target in binary format (ex: 111111001) for mining the initial overlay")
	cmd.Flags().String(optionNameNeighborhoodSuggester, "https://api.swarmscan.io/v1/network/neighborhoods/suggestion", "suggester for target neighborhood")
	cmd.Flags().StringSlice(optionNameWhitelistedWithdrawalAddress, []string{}, "withdrawal target addresses")
}

func newLogger(cmd *cobra.Command, verbosity string) (log.Logger, error) {
	var (
		sink   = cmd.OutOrStdout()
		vLevel = log.VerbosityNone
	)

	switch verbosity {
	case "0", "silent":
		sink = io.Discard
	case "1", "error":
		vLevel = log.VerbosityError
	case "2", "warn":
		vLevel = log.VerbosityWarning
	case "3", "info":
		vLevel = log.VerbosityInfo
	case "4", "debug":
		vLevel = log.VerbosityDebug
	case "5", "trace":
		vLevel = log.VerbosityDebug + 1 // For backwards compatibility, just enable v1 debugging as trace.
	default:
		return nil, fmt.Errorf("unknown verbosity level %q", verbosity)
	}

	log.ModifyDefaults(
		log.WithTimestamp(),
		log.WithLogMetrics(),
	)

	return log.NewLogger(
		node.LoggerName,
		log.WithSink(sink),
		log.WithVerbosity(vLevel),
	).Register(), nil
}

func (c *command) CheckUnknownParams(cmd *cobra.Command, args []string) error {
	if err := c.initConfig(); err != nil {
		return err
	}
	var unknownParams []string
	for _, v := range c.config.AllKeys() {
		if cmd.Flags().Lookup(v) == nil {
			unknownParams = append(unknownParams, v)
		}
	}

	if len(unknownParams) > 0 {
		return fmt.Errorf("unknown parameters:\n\t%v", strings.Join(unknownParams, "\n\t"))
	}

	return nil
}
