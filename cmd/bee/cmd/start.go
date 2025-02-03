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
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ethersphere/bee/v2"
	"github.com/ethersphere/bee/v2/pkg/accesscontrol"
	chaincfg "github.com/ethersphere/bee/v2/pkg/config"
	"github.com/ethersphere/bee/v2/pkg/crypto"
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

			v := strings.ToLower(c.config.GetString(optionNameVerbosity))

			logger, err := newLogger(cmd, v)
			if err != nil {
				return fmt.Errorf("new logger: %w", err)
			}

			if c.isWindowsService {
				logger, err = createWindowsEventLogger(serviceName, logger)
				if err != nil {
					return fmt.Errorf("failed to create windows logger %w", err)
				}
			}

			fmt.Print(beeWelcomeMessage)
			time.Sleep(5 * time.Second)
			fmt.Printf("\n\nversion: %v - planned to be supported until %v, please follow https://ethswarm.org/\n\n", bee.Version, endSupportDate())
			logger.Info("bee version", "version", bee.Version)

			go startTimeBomb(logger)

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
			respC := buildBeeNodeAsync(ctx, c, cmd, logger)
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
			return c.config.BindPFlags(cmd.Flags())
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

func buildBeeNodeAsync(ctx context.Context, c *command, cmd *cobra.Command, logger log.Logger) <-chan buildBeeNodeResp {
	respC := make(chan buildBeeNodeResp, 1)

	go func() {
		bee, err := buildBeeNode(ctx, c, cmd, logger)
		respC <- buildBeeNodeResp{bee, err}
	}()

	return respC
}

func buildBeeNode(ctx context.Context, c *command, cmd *cobra.Command, logger log.Logger) (*node.Bee, error) {
	var err error

	// If the resolver is specified, resolve all connection strings
	// and fail on any errors.
	var resolverCfgs []multiresolver.ConnectionConfig
	resolverEndpoints := c.config.GetStringSlice(optionNameResolverEndpoints)
	if len(resolverEndpoints) > 0 {
		resolverCfgs, err = multiresolver.ParseConnectionStrings(resolverEndpoints)
		if err != nil {
			return nil, err
		}
	}

	signerConfig, err := c.configureSigner(cmd, logger)
	if err != nil {
		return nil, err
	}

	bootNode := c.config.GetBool(optionNameBootnodeMode)
	fullNode := c.config.GetBool(optionNameFullNode)

	if bootNode && !fullNode {
		return nil, errors.New("boot node must be started as a full node")
	}

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

	tracingEndpoint := c.config.GetString(optionNameTracingEndpoint)

	if c.config.IsSet(optionNameTracingHost) && c.config.IsSet(optionNameTracingPort) {
		tracingEndpoint = strings.Join([]string{c.config.GetString(optionNameTracingHost), c.config.GetString(optionNameTracingPort)}, ":")
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

	swapEndpoint := c.config.GetString(optionNameSwapEndpoint)
	blockchainRpcEndpoint := c.config.GetString(optionNameBlockchainRpcEndpoint)
	if swapEndpoint != "" {
		blockchainRpcEndpoint = swapEndpoint
	}

	var neighborhoodSuggester string
	if networkID == chaincfg.Mainnet.NetworkID {
		neighborhoodSuggester = c.config.GetString(optionNameNeighborhoodSuggester)
	}

	b, err := node.NewBee(ctx, c.config.GetString(optionNameP2PAddr), signerConfig.publicKey, signerConfig.signer, networkID, logger, signerConfig.libp2pPrivateKey, signerConfig.pssPrivateKey, signerConfig.session, &node.Options{
		DataDir:                       c.config.GetString(optionNameDataDir),
		CacheCapacity:                 c.config.GetUint64(optionNameCacheCapacity),
		DBOpenFilesLimit:              c.config.GetUint64(optionNameDBOpenFilesLimit),
		DBBlockCacheCapacity:          c.config.GetUint64(optionNameDBBlockCacheCapacity),
		DBWriteBufferSize:             c.config.GetUint64(optionNameDBWriteBufferSize),
		DBDisableSeeksCompaction:      c.config.GetBool(optionNameDBDisableSeeksCompaction),
		APIAddr:                       c.config.GetString(optionNameAPIAddr),
		Addr:                          c.config.GetString(optionNameP2PAddr),
		NATAddr:                       c.config.GetString(optionNameNATAddr),
		EnableWS:                      c.config.GetBool(optionNameP2PWSEnable),
		WelcomeMessage:                c.config.GetString(optionWelcomeMessage),
		Bootnodes:                     networkConfig.bootNodes,
		CORSAllowedOrigins:            c.config.GetStringSlice(optionCORSAllowedOrigins),
		TracingEnabled:                c.config.GetBool(optionNameTracingEnabled),
		TracingEndpoint:               tracingEndpoint,
		TracingServiceName:            c.config.GetString(optionNameTracingServiceName),
		Logger:                        logger,
		PaymentThreshold:              c.config.GetString(optionNamePaymentThreshold),
		PaymentTolerance:              c.config.GetInt64(optionNamePaymentTolerance),
		PaymentEarly:                  c.config.GetInt64(optionNamePaymentEarly),
		ResolverConnectionCfgs:        resolverCfgs,
		BootnodeMode:                  bootNode,
		BlockchainRpcEndpoint:         blockchainRpcEndpoint,
		SwapFactoryAddress:            c.config.GetString(optionNameSwapFactoryAddress),
		SwapInitialDeposit:            c.config.GetString(optionNameSwapInitialDeposit),
		SwapEnable:                    c.config.GetBool(optionNameSwapEnable),
		ChequebookEnable:              c.config.GetBool(optionNameChequebookEnable),
		FullNodeMode:                  fullNode,
		PostageContractAddress:        c.config.GetString(optionNamePostageContractAddress),
		PostageContractStartBlock:     c.config.GetUint64(optionNamePostageContractStartBlock),
		PriceOracleAddress:            c.config.GetString(optionNamePriceOracleAddress),
		RedistributionContractAddress: c.config.GetString(optionNameRedistributionAddress),
		StakingContractAddress:        c.config.GetString(optionNameStakingAddress),
		BlockTime:                     networkConfig.blockTime,
		DeployGasPrice:                c.config.GetString(optionNameSwapDeploymentGasPrice),
		WarmupTime:                    c.config.GetDuration(optionWarmUpTime),
		ChainID:                       networkConfig.chainID,
		RetrievalCaching:              c.config.GetBool(optionNameRetrievalCaching),
		Resync:                        c.config.GetBool(optionNameResync),
		BlockProfile:                  c.config.GetBool(optionNamePProfBlock),
		MutexProfile:                  c.config.GetBool(optionNamePProfMutex),
		StaticNodes:                   staticNodes,
		AllowPrivateCIDRs:             c.config.GetBool(optionNameAllowPrivateCIDRs),
		UsePostageSnapshot:            c.config.GetBool(optionNameUsePostageSnapshot),
		EnableStorageIncentives:       c.config.GetBool(optionNameStorageIncentivesEnable),
		StatestoreCacheCapacity:       c.config.GetUint64(optionNameStateStoreCacheCapacity),
		TargetNeighborhood:            c.config.GetString(optionNameTargetNeighborhood),
		NeighborhoodSuggester:         neighborhoodSuggester,
		WhitelistedWithdrawalAddress:  c.config.GetStringSlice(optionNameWhitelistedWithdrawalAddress),
		TrxDebugMode:                  c.config.GetBool(optionNameTransactionDebugMode),
		MinimumStorageRadius:          c.config.GetUint(optionMinimumStorageRadius),
		ReserveCapacityDoubling:       c.config.GetInt(optionReserveCapacityDoubling),
	})

	return b, err
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
