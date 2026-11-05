// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2"
	"github.com/ethersphere/bee/v2/pkg/accesscontrol"
	"github.com/ethersphere/bee/v2/pkg/bmt"
	"github.com/ethersphere/bee/v2/pkg/bmtpool"
	chaincfg "github.com/ethersphere/bee/v2/pkg/config"
	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/keccak"
	"github.com/ethersphere/bee/v2/pkg/keystore"
	filekeystore "github.com/ethersphere/bee/v2/pkg/keystore/file"
	memkeystore "github.com/ethersphere/bee/v2/pkg/keystore/mem"
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/node"
	"github.com/ethersphere/bee/v2/pkg/resolver/multiresolver"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/kardianos/service"
	"github.com/spf13/cobra"
)

const (
	serviceName      = "SwarmBeeSvc"
	libp2pPKFilename = "libp2p_v2"
)

//go:embed bee-welcome-message.txt
var beeWelcomeMessage string

func (c *command) initStartCmd() (err error) {
	cmd := &cobra.Command{
		Use:               "start",
		Short:             "Start a Swarm node",
		PersistentPreRunE: c.CheckUnknownParams,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if len(args) > 0 {
				return cmd.Help()
			}

			logger := c.logger

			fmt.Print(beeWelcomeMessage)
			logger.Info("bee version", "version", bee.Version)

			// Resolve the node mode before the node is built, so that an invalid
			// configuration exits with a non-zero status, and before the keystore
			// is unlocked.
			nodeMode, err := c.resolveNodeMode(logger)
			if err != nil {
				return err
			}

			// ctx is global context of bee node; which is canceled after interrupt signal is received.
			ctx, cancel := context.WithCancel(context.Background())
			sysInterruptChannel := make(chan os.Signal, 1)
			signal.Notify(sysInterruptChannel, syscall.SIGINT, syscall.SIGTERM)

			go func() {
				select {
				case <-sysInterruptChannel:
					logger.Info("received interrupt signal")
					cancel()
				case <-ctx.Done():
				}
			}()

			// Building bee node can take up some time (because node.NewBee(...) is compute have function )
			// Because of this we need to do it in background so that program could be terminated when interrupt signal is received
			// while bee node is being constructed.
			respC := buildBeeNodeAsync(ctx, c, cmd, logger, nodeMode)
			var beeNode atomic.Value

			p := &program{
				start: func() {
					// Wait for bee node to fully build and initialized
					select {
					case resp := <-respC:
						if resp.err != nil {
							logger.Error(resp.err, "failed to build bee node")
							return
						}
						beeNode.Store(resp.bee)
					case <-ctx.Done():
						return
					}

					// Bee has fully started at this point, from now on we
					// block main goroutine until it is interrupted or stopped
					select {
					case <-ctx.Done():
					case <-beeNode.Load().(*node.Bee).SyncingStopped():
						logger.Debug("syncing has stopped")
					}

					logger.Info("shutting down...")
				},
				stop: func() {
					// Whenever program is being stopped we need to cancel main context
					// beforehand so that node could be stopped via Shutdown method
					cancel()

					// Shutdown node (if node was fully started)
					val := beeNode.Load()
					if val == nil {
						return
					}

					done := make(chan struct{})
					go func(beeNode *node.Bee) {
						defer close(done)

						if err := beeNode.Shutdown(); err != nil {
							logger.Error(err, "shutdown failed")
						}
					}(val.(*node.Bee))

					// If shutdown function is blocking too long,
					// allow process termination by receiving another signal.
					select {
					case <-sysInterruptChannel:
						logger.Info("node shutdown terminated")
					case <-done:
						logger.Info("node shutdown")
					}
				},
			}

			if c.isWindowsService {
				s, err := service.New(p, &service.Config{
					Name:        serviceName,
					DisplayName: "Bee",
					Description: "Bee, Swarm client.",
				})
				if err != nil {
					return err
				}

				if err = s.Run(); err != nil {
					return err
				}
			} else {
				// start blocks until some interrupt is received
				p.start()
				p.stop()
			}

			return nil
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := c.preRun(cmd); err != nil {
				return err
			}
			c.bindBlockchainRpcConfig(cmd)
			c.bindTracingConfig(cmd)
			c.warnDeprecatedTracingKeys()
			c.warnTracingTLSWithoutCA()
			return nil
		},
	}

	c.setAllFlags(cmd)
	c.root.AddCommand(cmd)
	return nil
}

type buildBeeNodeResp struct {
	bee *node.Bee
	err error
}

func buildBeeNodeAsync(ctx context.Context, c *command, cmd *cobra.Command, logger log.Logger, nodeMode node.NodeMode) <-chan buildBeeNodeResp {
	respC := make(chan buildBeeNodeResp, 1)

	go func() {
		bee, err := buildBeeNode(ctx, c, cmd, logger, nodeMode)
		respC <- buildBeeNodeResp{bee, err}
	}()

	return respC
}

func buildBeeNode(ctx context.Context, c *command, cmd *cobra.Command, logger log.Logger, nodeMode node.NodeMode) (*node.Bee, error) {
	var err error

	// If the resolver is specified, resolve all connection strings
	// and fail on any errors.
	var resolverCfgs []multiresolver.ConnectionConfig
	resolverEndpoints := c.config.GetStringSlice(optionNameResolverEndpoints)
	if len(resolverEndpoints) > 0 {
		resolverCfgs, err = multiresolver.ParseConnectionStrings(resolverEndpoints)
		if err != nil {
			return nil, fmt.Errorf("parse resolver endpoints: %w", err)
		}
	}

	signerConfig, err := c.configureSigner(cmd, logger)
	if err != nil {
		return nil, fmt.Errorf("configure signer: %w", err)
	}

	bootNode := c.config.GetBool(optionNameBootnodeMode)

	mainnet := c.config.GetBool(optionNameMainNet)
	userHasSetNetworkID := c.config.IsSet(optionNameNetworkID)

	// if the user has provided a value - we use it and overwrite the default
	// if mainnet is true then we only accept networkID value 1, error otherwise
	// if the user has not provided a network ID but mainnet is true - just overwrite with mainnet network ID (1)
	// in all the other cases we default to test network ID (10)
	networkID := chaincfg.Testnet.NetworkID

	if userHasSetNetworkID {
		networkID = c.config.GetUint64(optionNameNetworkID)
		if mainnet && networkID != chaincfg.Mainnet.NetworkID {
			return nil, errors.New("provided network ID does not match mainnet")
		}
	} else if mainnet {
		networkID = chaincfg.Mainnet.NetworkID
	}

	bootnodes := c.config.GetStringSlice(optionNameBootnodes)
	blockTime := c.config.GetUint64(optionNameBlockTime)

	networkConfig := getConfigByNetworkID(networkID, blockTime)

	if c.config.IsSet(optionNameBootnodes) {
		networkConfig.bootNodes = bootnodes
	}

	if c.config.IsSet(optionNameBlockTime) && blockTime != 0 {
		networkConfig.blockTime = time.Duration(blockTime) * time.Second
	}

	staticNodesOpt := c.config.GetStringSlice(optionNameStaticNodes)
	staticNodes := make([]swarm.Address, 0, len(staticNodesOpt))
	for _, p := range staticNodesOpt {
		addr, err := swarm.ParseHexAddress(p)
		if err != nil {
			return nil, fmt.Errorf("invalid swarm address %q configured for static node", p)
		}

		staticNodes = append(staticNodes, addr)
	}
	if len(staticNodes) > 0 && !bootNode {
		return nil, errors.New("static nodes can only be configured on bootnodes")
	}

	var neighborhoodSuggester string
	if networkID == chaincfg.Mainnet.NetworkID {
		neighborhoodSuggester = c.config.GetString(optionNameNeighborhoodSuggester)
	}

	useSIMD := c.config.GetBool(optionUseSIMD)
	if useSIMD {
		if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
			return nil, fmt.Errorf("SIMD hashing requires linux/amd64 (this build is %s/%s)", runtime.GOOS, runtime.GOARCH)
		}
		if !keccak.HasSIMD() {
			return nil, errors.New("SIMD hashing requires a CPU with AVX2 or AVX-512; this CPU has neither")
		}
		bmt.SetSIMDOptIn(true)
		// Rebuild the global bmtpool instance so the new SIMDOptIn value
		// is reflected in the pool created for hot-path BMT hashing.
		bmtpool.Rebuild()
		logger.Info("SIMD hashing enabled", "batch_width", keccak.BatchWidth(), "avx512", keccak.HasAVX512())
	}

	var bzzTokenAddress common.Address
	if a := c.config.GetString(optionNameBzzTokenAddress); a != "" {
		if !common.IsHexAddress(a) {
			return nil, errors.New("malformed bzz token address")
		}
		bzzTokenAddress = common.HexToAddress(a)
	}

	b, err := node.NewBee(ctx, c.config.GetString(optionNameP2PAddr), signerConfig.publicKey, signerConfig.signer, networkID, logger, signerConfig.libp2pPrivateKey, signerConfig.pssPrivateKey, signerConfig.session, &node.Options{
		Addr:                          c.config.GetString(optionNameP2PAddr),
		AllowPrivateCIDRs:             c.config.GetBool(optionNameAllowPrivateCIDRs),
		APIAddr:                       c.config.GetString(optionNameAPIAddr),
		EnableWSS:                     c.config.GetBool(optionNameP2PWSSEnable),
		WSSAddr:                       c.config.GetString(optionP2PWSSAddr),
		AutoTLSStorageDir:             filepath.Join(c.config.GetString(optionNameDataDir), "autotls"),
		BlockchainRpcEndpoint:         c.config.GetString(configKeyBlockchainRpcEndpoint),
		BlockchainRpcDialTimeout:      c.config.GetDuration(configKeyBlockchainRpcDialTimeout),
		BlockchainRpcTLSTimeout:       c.config.GetDuration(configKeyBlockchainRpcTLSTimeout),
		BlockchainRpcIdleTimeout:      c.config.GetDuration(configKeyBlockchainRpcIdleTimeout),
		BlockchainRpcKeepalive:        c.config.GetDuration(configKeyBlockchainRpcKeepalive),
		BzzTokenAddress:               bzzTokenAddress,
		BlockProfile:                  c.config.GetBool(optionNamePProfBlock),
		BlockTime:                     networkConfig.blockTime,
		BlockSyncInterval:             c.config.GetUint64(optionNameBlockSyncInterval),
		BootnodeMode:                  bootNode,
		Bootnodes:                     networkConfig.bootNodes,
		CacheCapacity:                 c.config.GetUint64(optionNameCacheCapacity),
		AutoTLSCAEndpoint:             c.config.GetString(optionAutoTLSCAEndpoint),
		ChainID:                       networkConfig.chainID,
		ChequebookEnable:              c.config.GetBool(optionNameChequebookEnable),
		ChequebookVerification:        c.config.GetBool(optionNameChequebookVerification),
		ChequebookMinBalance:          c.config.GetString(optionNameChequebookMinBalance),
		CORSAllowedOrigins:            c.config.GetStringSlice(optionCORSAllowedOrigins),
		DataDir:                       c.config.GetString(optionNameDataDir),
		DBBlockCacheCapacity:          c.config.GetUint64(optionNameDBBlockCacheCapacity),
		DBDisableSeeksCompaction:      c.config.GetBool(optionNameDBDisableSeeksCompaction),
		DBOpenFilesLimit:              c.config.GetUint64(optionNameDBOpenFilesLimit),
		DBWriteBufferSize:             c.config.GetUint64(optionNameDBWriteBufferSize),
		EnableStorageIncentives:       c.config.GetBool(optionNameStorageIncentivesEnable),
		EnableWS:                      c.config.GetBool(optionNameP2PWSEnable),
		AutoTLSDomain:                 c.config.GetString(optionAutoTLSDomain),
		AutoTLSRegistrationEndpoint:   c.config.GetString(optionAutoTLSRegistrationEndpoint),
		NodeMode:                      nodeMode,
		LightNodeLimit:                c.config.GetInt(optionNameLightNodeLimit),
		Logger:                        logger,
		MinimumGasTipCap:              c.config.GetUint64(optionNameMinimumGasTipCap),
		GasLimitFallback:              c.config.GetUint64(optionNameGasLimitFallback),
		MinimumStorageRadius:          c.config.GetUint(optionMinimumStorageRadius),
		MutexProfile:                  c.config.GetBool(optionNamePProfMutex),
		NATAddr:                       c.config.GetString(optionNameNATAddr),
		NATWSSAddr:                    c.config.GetString(optionNATWSSAddr),
		NeighborhoodSuggester:         neighborhoodSuggester,
		PaymentEarly:                  c.config.GetInt64(optionNamePaymentEarly),
		PaymentThreshold:              c.config.GetString(optionNamePaymentThreshold),
		PaymentTolerance:              c.config.GetInt64(optionNamePaymentTolerance),
		PostageContractAddress:        c.config.GetString(optionNamePostageContractAddress),
		PostageContractStartBlock:     c.config.GetUint64(optionNamePostageContractStartBlock),
		PostageSyncBlockRange:         c.config.GetUint64(optionNamePostageSyncBlockRange),
		PriceOracleAddress:            c.config.GetString(optionNamePriceOracleAddress),
		RedistributionContractAddress: c.config.GetString(optionNameRedistributionAddress),
		ReserveCapacityDoubling:       c.config.GetInt(optionReserveCapacityDoubling),
		ResolverConnectionCfgs:        resolverCfgs,
		Resync:                        c.config.GetBool(optionNameResync),
		RetrievalCaching:              c.config.GetBool(optionNameRetrievalCaching),
		SkipPostageSnapshot:           c.config.GetBool(optionSkipPostageSnapshot),
		StakingContractAddress:        c.config.GetString(optionNameStakingAddress),
		StatestoreCacheCapacity:       c.config.GetUint64(optionNameStateStoreCacheCapacity),
		StaticNodes:                   staticNodes,
		SwapEnable:                    c.config.GetBool(optionNameSwapEnable),
		SwapFactoryAddress:            c.config.GetString(optionNameSwapFactoryAddress),
		SwapInitialDeposit:            c.config.GetString(optionNameSwapInitialDeposit),
		TargetNeighborhood:            c.config.GetString(optionNameTargetNeighborhood),
		TracingEnabled:                c.config.GetBool(configKeyTracingEnabled),
		TracingEndpoint:               c.config.GetString(configKeyTracingEndpoint),
		TracingInsecure:               c.config.GetBool(configKeyTracingInsecure),
		TracingCAFile:                 c.config.GetString(configKeyTracingCAFile),
		TracingProtocol:               c.config.GetString(configKeyTracingProtocol),
		TracingSamplingRatio:          c.config.GetFloat64(configKeyTracingSamplingRatio),
		TracingServiceName:            c.config.GetString(configKeyTracingServiceName),
		TrxDebugMode:                  c.config.GetBool(optionNameTransactionDebugMode),
		WarmupTime:                    c.config.GetDuration(optionWarmUpTime),
		WelcomeMessage:                c.config.GetString(optionWelcomeMessage),
		WhitelistedWithdrawalAddress:  c.config.GetStringSlice(optionNameWhitelistedWithdrawalAddress),
	})

	return b, err
}

// legacyNodeModeRemovalVersion is the release in which node-mode inference from
// the deprecated full-node option and blockchain-rpc-endpoint presence is
// removed. From that release on, node-mode is required.
const legacyNodeModeRemovalVersion = "v2.11.0"

// resolveNodeMode determines the effective node mode from config. There are
// two regimes, selected by whether node-mode is set.
//
// node-mode unset (legacy): the node behaves as releases before node-mode
// existed. The mode is inferred from the deprecated full-node option and the
// presence of blockchain-rpc-endpoint, and the flag defaults of true for
// chequebook-enable and storage-incentives-enable stay in effect. A deprecation
// warning lists the config changes that keep the node's behavior once
// node-mode is set, and the release in which the inference is removed.
//
// node-mode set: the mode owns the config. Options the mode implies are
// enabled unless the operator explicitly disabled them (full bootnodes are
// exempt), options the mode cannot support are rejected when explicitly enabled
// (overriding the pflag defaults of true when unset), and
// blockchain-rpc-endpoint is required for light and full nodes. An unset
// chequebook-enable follows swap-enable. Implied values are written back to
// config so the rest of node startup picks them up.
//
// In both regimes, options the resolved mode cannot run with are rejected
// before the node is built.
func (c *command) resolveNodeMode(logger log.Logger) (node.NodeMode, error) {
	raw := c.config.GetString(optionNameNodeMode)
	legacy := raw == ""

	var (
		mode node.NodeMode
		err  error
	)
	if legacy {
		mode, err = c.resolveLegacyNodeMode()
	} else {
		mode, err = c.resolveExplicitNodeMode(logger, raw)
	}
	if err != nil {
		return "", err
	}
	if err := c.validateNodeModeOptions(mode); err != nil {
		return "", err
	}
	if legacy {
		c.logLegacyNodeModeMigration(logger, mode)
	}
	return mode, nil
}

// resolveExplicitNodeMode applies the regime in which node-mode is set. The
// value is matched case-insensitively, ignoring surrounding whitespace.
func (c *command) resolveExplicitNodeMode(logger log.Logger, raw string) (node.NodeMode, error) {
	mode := node.NodeMode(strings.ToLower(strings.TrimSpace(raw)))
	if !mode.IsValid() {
		return "", fmt.Errorf("invalid node-mode %q: must be one of full, light, ultra-light", raw)
	}
	if c.config.GetBool(optionNameFullNode) {
		logger.Warning("--full-node is set alongside --node-mode; --full-node is ignored")
	}

	// Surrounding whitespace is not significant: a whitespace-only endpoint
	// counts as unset, and the trimmed value is what the node dials.
	rpcEndpoint := strings.TrimSpace(c.config.GetString(configKeyBlockchainRpcEndpoint))
	if rpcEndpoint != c.config.GetString(configKeyBlockchainRpcEndpoint) {
		c.config.Set(configKeyBlockchainRpcEndpoint, rpcEndpoint)
	}

	switch mode {
	case node.FullMode:
		if rpcEndpoint == "" {
			return "", errors.New("full node requires blockchain-rpc-endpoint to be set")
		}
		// A full node implies swap, chequebook and storage incentives. Bootnodes
		// are exempt: NewBee never runs the reserve or the incentives agent for
		// them, so they have no use for the implied options. Swap still starts on
		// a bootnode when explicitly enabled, and chequebook follows it.
		if !c.config.GetBool(optionNameBootnodeMode) {
			c.enableImpliedOption(logger, mode, optionNameSwapEnable)
			if c.config.GetBool(optionNameSwapEnable) {
				c.enableImpliedOption(logger, mode, optionNameChequebookEnable)
			}
			c.enableImpliedOption(logger, mode, optionNameStorageIncentivesEnable)
		} else if !c.config.IsSet(optionNameStorageIncentivesEnable) {
			c.config.Set(optionNameStorageIncentivesEnable, false)
		} else if c.config.GetBool(optionNameStorageIncentivesEnable) {
			logger.Warning("storage-incentives-enable has no effect on a bootnode", "node_mode", mode)
		}
	case node.LightMode:
		if rpcEndpoint == "" {
			return "", errors.New("light node requires blockchain-rpc-endpoint to be set")
		}
		// Storage incentives are full-node only; override the pflag default of true when unset.
		if !c.config.IsSet(optionNameStorageIncentivesEnable) {
			c.config.Set(optionNameStorageIncentivesEnable, false)
		} else if c.config.GetBool(optionNameStorageIncentivesEnable) {
			return "", errors.New("light node cannot have storage-incentives-enable set to true")
		}
		// Light mode does not imply swap, as before node-mode. Without it the
		// node pays peers only through the free pseudosettle allowance, which
		// limits uploads and downloads.
		if !c.config.GetBool(optionNameSwapEnable) {
			logger.Info("swap is disabled, so uploads and downloads are limited to the free pseudosettle allowance; to exceed it set swap-enable=true and fund the chequebook", "node_mode", mode)
		}
	case node.UltraLightMode:
		if rpcEndpoint != "" {
			logger.Warning("blockchain-rpc-endpoint is ignored in ultra-light mode", "node_mode", mode)
		}
		if c.config.GetBool(optionNameSwapEnable) {
			return "", errors.New("ultra-light node cannot have swap-enable set to true")
		}
		// Storage incentives are not supported on ultra-light; override the pflag default of true when unset.
		if !c.config.IsSet(optionNameStorageIncentivesEnable) {
			c.config.Set(optionNameStorageIncentivesEnable, false)
		} else if c.config.GetBool(optionNameStorageIncentivesEnable) {
			return "", errors.New("ultra-light node cannot have storage-incentives-enable set to true")
		}
		if c.config.IsSet(optionNameChequebookEnable) && c.config.GetBool(optionNameChequebookEnable) {
			return "", errors.New("ultra-light node cannot have chequebook-enable set to true")
		}
	}

	// Chequebook requires swap. When chequebook is not explicitly configured,
	// follow swap rather than the pflag default of true.
	if !c.config.IsSet(optionNameChequebookEnable) {
		c.config.Set(optionNameChequebookEnable, c.config.GetBool(optionNameSwapEnable))
	}

	// chequebook init is gated on swap-enable in NewBee. With node-mode set the
	// implied values above never produce this combination, so it is always an
	// explicit contradiction rather than a silent no-op.
	if c.config.GetBool(optionNameChequebookEnable) && !c.config.GetBool(optionNameSwapEnable) {
		return "", errors.New("chequebook-enable requires swap-enable to be true")
	}

	return mode, nil
}

// enableImpliedOption turns on an option implied by the node mode unless the
// operator set it explicitly. An explicit false is honored and reported at Info:
// it is a deliberate choice (the legacy migration may suggest it), not a
// problem for the operator to act on.
func (c *command) enableImpliedOption(logger log.Logger, mode node.NodeMode, key string) {
	if !c.config.IsSet(key) {
		logger.Debug("enabling option implied by node-mode", "node_mode", mode, "option", key)
		c.config.Set(key, true)
		return
	}
	if !c.config.GetBool(key) {
		logger.Info("option implied by node-mode is explicitly disabled", "node_mode", mode, "option", key)
	}
}

// resolveLegacyNodeMode infers the mode as releases before node-mode did, for
// configs that do not set node-mode. A full node must have a
// blockchain-rpc-endpoint: those releases enabled the chain backend for every
// full node and failed at chain init without one, so the early error changes
// the message, not the outcome.
func (c *command) resolveLegacyNodeMode() (node.NodeMode, error) {
	switch {
	case c.config.GetBool(optionNameFullNode):
		if strings.TrimSpace(c.config.GetString(configKeyBlockchainRpcEndpoint)) == "" {
			return "", errors.New("full node requires blockchain-rpc-endpoint to be set")
		}
		return node.FullMode, nil
	case c.config.GetString(configKeyBlockchainRpcEndpoint) != "":
		// Untrimmed, as before node-mode existed: a whitespace-only endpoint
		// still selects light mode and fails at chain init instead of silently
		// starting an ultra-light node.
		return node.LightMode, nil
	default:
		return node.UltraLightMode, nil
	}
}

// validateNodeModeOptions rejects options the resolved mode cannot run with, in
// both regimes. Releases before node-mode rejected the same configurations,
// the bootnode one in the start command and the others in NewBee, which keeps
// its checks for other callers. Checking here rejects the configuration before
// the node is built, with a non-zero exit status.
func (c *command) validateNodeModeOptions(mode node.NodeMode) error {
	chainEnabled := mode == node.FullMode || mode == node.LightMode
	switch {
	case c.config.GetBool(optionNameBootnodeMode) && mode != node.FullMode:
		return errors.New("boot node must be started as a full node")
	case mode != node.FullMode && c.config.GetInt(optionReserveCapacityDoubling) != 0:
		return errors.New("reserve capacity doubling is only allowed for full nodes")
	case c.config.GetBool(optionNameSwapEnable) && !chainEnabled:
		return errors.New("swap is enabled but the chain backend is not; provide --blockchain-rpc-endpoint or disable swap")
	case c.config.GetBool(optionNameChequebookVerification) && (mode != node.FullMode || !c.config.GetBool(optionNameChequebookEnable) || !chainEnabled):
		return fmt.Errorf("chequebook-verification requires full-node mode, chequebook-enable, and an enabled chain backend (full_node=%t, chequebook_enable=%t, chain_enabled=%t)",
			mode == node.FullMode, c.config.GetBool(optionNameChequebookEnable), chainEnabled)
	}
	return nil
}

// legacyNodeModeMigration lists the config changes that make a legacy config
// behave the same once node-mode is set to the inferred mode, as option=value
// pairs to set and options to remove. Notes describe behavior that cannot be
// preserved.
type legacyNodeModeMigration struct {
	set    []string
	remove []string
	notes  []string
}

func (c *command) legacyMigration(mode node.NodeMode) legacyNodeModeMigration {
	var m legacyNodeModeMigration
	bootnode := c.config.GetBool(optionNameBootnodeMode)
	swap := c.config.GetBool(optionNameSwapEnable)
	verification := c.config.GetBool(optionNameChequebookVerification)

	m.set = append(m.set, optionNameNodeMode+"="+string(mode))
	if c.config.IsSet(optionNameFullNode) {
		m.remove = append(m.remove, optionNameFullNode)
	}
	// With node-mode set, chequebook-enable follows swap-enable and
	// chequebook-verification requires the chequebook, so verification cannot
	// keep running without swap.
	switch {
	case verification && !swap && mode == node.FullMode && !bootnode && !c.config.IsSet(optionNameSwapEnable):
		m.notes = append(m.notes, "node-mode: full enables swap and chequebook, which chequebook-verification requires; to keep swap off, set swap-enable=false and remove chequebook-verification")
	case verification && !swap:
		m.notes = append(m.notes, "chequebook-verification requires swap-enable once node-mode is set; set swap-enable=true or remove chequebook-verification")
	case mode == node.FullMode && !bootnode && !c.config.IsSet(optionNameSwapEnable):
		// node-mode: full implies swap, which legacy full nodes did not.
		m.set = append(m.set, optionNameSwapEnable+"=false")
	}
	// With node-mode set, an explicit chequebook-enable=true is rejected
	// without swap, and on ultra-light nodes.
	if c.config.IsSet(optionNameChequebookEnable) && c.config.GetBool(optionNameChequebookEnable) && (mode == node.UltraLightMode || !swap) {
		m.remove = append(m.remove, optionNameChequebookEnable)
	}
	// Legacy inference treats a whitespace-only endpoint as set; with node-mode
	// set it counts as unset, which light mode rejects.
	if mode == node.LightMode && strings.TrimSpace(c.config.GetString(configKeyBlockchainRpcEndpoint)) == "" {
		m.notes = append(m.notes, "blockchain-rpc-endpoint contains only whitespace; node-mode=light requires an endpoint")
	}
	// Storage incentives only run on full nodes that are not bootnodes; with
	// node-mode set an explicit true is rejected on light and ultra-light nodes.
	if c.config.IsSet(optionNameStorageIncentivesEnable) && c.config.GetBool(optionNameStorageIncentivesEnable) && (mode != node.FullMode || bootnode) {
		m.remove = append(m.remove, optionNameStorageIncentivesEnable)
	}
	return m
}

// logLegacyNodeModeMigration warns that the mode was inferred and lists the
// config changes that keep the node's behavior once node-mode is set.
func (c *command) logLegacyNodeModeMigration(logger log.Logger, mode node.NodeMode) {
	m := c.legacyMigration(mode)
	kv := []any{
		"node_mode", mode,
		"removed_in", legacyNodeModeRemovalVersion,
		"set", m.set,
	}
	if len(m.remove) > 0 {
		kv = append(kv, "remove", m.remove)
	}
	if len(m.notes) > 0 {
		kv = append(kv, "behavior_change", m.notes)
	}
	logger.Warning("node-mode is not set and was inferred from legacy options, which will be removed; to keep the current behavior, apply the listed options in the config file, flags or BEE_* environment variables", kv...)
}

type program struct {
	start func()
	stop  func()
}

func (p *program) Start(s service.Service) error {
	// Start should not block. Do the actual work async.
	go p.start()
	return nil
}

func (p *program) Stop(s service.Service) error {
	p.stop()
	return nil
}

type signerConfig struct {
	signer           crypto.Signer
	publicKey        *ecdsa.PublicKey
	libp2pPrivateKey *ecdsa.PrivateKey
	pssPrivateKey    *ecdsa.PrivateKey
	session          accesscontrol.Session
}

func (c *command) configureSigner(cmd *cobra.Command, logger log.Logger) (config *signerConfig, err error) {
	var keystore keystore.Service
	if c.config.GetString(optionNameDataDir) == "" {
		keystore = memkeystore.New()
		logger.Warning("data directory not provided, keys are not persisted")
	} else {
		keystore = filekeystore.New(filepath.Join(c.config.GetString(optionNameDataDir), "keys"))
	}

	var signer crypto.Signer
	var password string
	var publicKey *ecdsa.PublicKey
	var session accesscontrol.Session
	if p := c.config.GetString(optionNamePassword); p != "" {
		password = p
	} else if pf := c.config.GetString(optionNamePasswordFile); pf != "" {
		b, err := os.ReadFile(pf)
		if err != nil {
			return nil, err
		}
		password = string(bytes.Trim(b, "\n"))
	} else {
		// if libp2p key exists we can assume all required keys exist
		// so prompt for a password to unlock them
		// otherwise prompt for new password with confirmation to create them
		exists, err := keystore.Exists(libp2pPKFilename)
		if err != nil {
			return nil, err
		}
		if exists {
			password, err = terminalPromptPassword(cmd, c.passwordReader, "Password")
			if err != nil {
				return nil, err
			}
		} else {
			password, err = terminalPromptCreatePassword(cmd, c.passwordReader)
			if err != nil {
				return nil, err
			}
		}
	}

	swarmPrivateKey, _, err := keystore.Key("swarm", password, crypto.EDGSecp256_K1)
	if err != nil {
		return nil, fmt.Errorf("swarm key: %w", err)
	}
	signer = crypto.NewDefaultSigner(swarmPrivateKey)
	publicKey = &swarmPrivateKey.PublicKey
	session = accesscontrol.NewDefaultSession(swarmPrivateKey)

	logger.Info("swarm public key", "public_key", hex.EncodeToString(crypto.EncodeSecp256k1PublicKey(publicKey)))

	libp2pPrivateKey, created, err := keystore.Key(libp2pPKFilename, password, crypto.EDGSecp256_R1)
	if err != nil {
		return nil, fmt.Errorf("libp2p v2 key: %w", err)
	}
	if created {
		logger.Debug("new libp2p v2 key created")
	} else {
		logger.Debug("using existing libp2p key")
	}

	pssPrivateKey, created, err := keystore.Key("pss", password, crypto.EDGSecp256_K1)
	if err != nil {
		return nil, fmt.Errorf("pss key: %w", err)
	}
	if created {
		logger.Debug("new pss key created")
	} else {
		logger.Debug("using existing pss key")
	}

	logger.Info("pss public key", "public_key", hex.EncodeToString(crypto.EncodeSecp256k1PublicKey(&pssPrivateKey.PublicKey)))

	// postinst and post scripts inside packaging/{deb,rpm} depend and parse on this log output
	overlayEthAddress, err := signer.EthereumAddress()
	if err != nil {
		return nil, err
	}
	logger.Info("using ethereum address", "address", overlayEthAddress)

	return &signerConfig{
		signer:           signer,
		publicKey:        publicKey,
		libp2pPrivateKey: libp2pPrivateKey,
		pssPrivateKey:    pssPrivateKey,
		session:          session,
	}, nil
}

type networkConfig struct {
	bootNodes []string
	blockTime time.Duration
	chainID   int64
}

func getConfigByNetworkID(networkID uint64, defaultBlockTimeInSeconds uint64) *networkConfig {
	config := networkConfig{
		blockTime: time.Duration(defaultBlockTimeInSeconds) * time.Second,
	}
	switch networkID {
	case chaincfg.Mainnet.NetworkID:
		config.bootNodes = []string{"/dnsaddr/mainnet.ethswarm.org"}
		config.blockTime = 5 * time.Second
		config.chainID = chaincfg.Mainnet.ChainID
	case 5: // Staging.
		config.chainID = chaincfg.Testnet.ChainID
	case chaincfg.Testnet.NetworkID:
		config.bootNodes = []string{"/dnsaddr/testnet.ethswarm.org"}
		config.blockTime = 12 * time.Second
		config.chainID = chaincfg.Testnet.ChainID
	default: // Will use the value provided by the chain.
		config.chainID = -1
	}

	return &config
}
