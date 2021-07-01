// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"io/ioutil"

	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/accounts/external"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethersphere/bee"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/crypto/clef"
	"github.com/ethersphere/bee/pkg/keystore"
	filekeystore "github.com/ethersphere/bee/pkg/keystore/file"
	memkeystore "github.com/ethersphere/bee/pkg/keystore/mem"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/node"
	"github.com/ethersphere/bee/pkg/resolver/multiresolver"
	"github.com/kardianos/service"
	"github.com/spf13/cobra"
)

const (
	serviceName = "SwarmBeeSvc"
)

func (c *command) initStartCmd() (err error) {

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a Swarm node",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if len(args) > 0 {
				return cmd.Help()
			}

			v := strings.ToLower(c.config.GetString(optionNameVerbosity))
			logger, err := newLogger(cmd, v)
			if err != nil {
				return fmt.Errorf("new logger: %v", err)
			}

			go startTimeBomb(logger)

			isWindowsService, err := isWindowsService()
			if err != nil {
				return fmt.Errorf("failed to determine if we are running in service: %w", err)
			}

			if isWindowsService {
				var err error
				logger, err = createWindowsEventLogger(serviceName, logger)
				if err != nil {
					return fmt.Errorf("failed to create windows logger %w", err)
				}
			}

			// If the resolver is specified, resolve all connection strings
			// and fail on any errors.
			var resolverCfgs []multiresolver.ConnectionConfig
			resolverEndpoints := c.config.GetStringSlice(optionNameResolverEndpoints)
			if len(resolverEndpoints) > 0 {
				resolverCfgs, err = multiresolver.ParseConnectionStrings(resolverEndpoints)
				if err != nil {
					return err
				}
			}

			beeASCII := `
Welcome to Swarm.... Bzzz Bzzzz Bzzzz
                \     /
            \    o ^ o    /
              \ (     ) /
   ____________(%%%%%%%)____________
  (     /   /  )%%%%%%%(  \   \     )
  (___/___/__/           \__\___\___)
     (     /  /(%%%%%%%)\  \     )
      (__/___/ (%%%%%%%) \___\__)
              /(       )\
            /   (%%%%%)   \
                 (%%%)
                   !                   `

			fmt.Println(beeASCII)
			fmt.Print(`
DISCLAIMER:
This software is provided to you "as is", use at your own risk and without warranties of any kind.
It is your responsibility to read and understand how Swarm works and the implications of running this software.
The usage of Bee involves various risks, including, but not limited to:
damage to hardware or loss of funds associated with the Ethereum account connected to your node.
No developers or entity involved will be liable for any claims and damages associated with your use,
inability to use, or your interaction with other nodes or the software.`)

			fmt.Printf("\n\nversion: %v - planned to be supported until %v, please follow http://ethswarm.org/\n\n", bee.Version, endSupportDate())

			debugAPIAddr := c.config.GetString(optionNameDebugAPIAddr)
			if !c.config.GetBool(optionNameDebugAPIEnable) {
				debugAPIAddr = ""
			}

			signerConfig, err := c.configureSigner(cmd, logger)
			if err != nil {
				return err
			}

			logger.Infof("version: %v", bee.Version)

			bootNode := c.config.GetBool(optionNameBootnodeMode)
			fullNode := c.config.GetBool(optionNameFullNode)

			if bootNode && !fullNode {
				return errors.New("boot node must be started as a full node")
			}

			mainnet := c.config.GetBool(optionNameMainNet)
			networkID := c.config.GetUint64(optionNameNetworkID)
			bootnodes := c.config.GetStringSlice(optionNameBootnodes)
			blockTime := c.config.GetUint64(optionNameBlockTime)

			if mainnet {
				userHasSetNetworkID := c.config.IsSet(optionNameNetworkID)
				if userHasSetNetworkID && networkID != 1 {
					return errors.New("provided network ID does not match mainnet")
				}
				networkID = 1
			}

			networkConfig := getDefaultNetworkConfig(networkID)

			if len(bootnodes) > 0 {
				networkConfig.bootNodes = bootnodes
			}
			if blockTime != 0 {
				networkConfig.blockTime = blockTime
			}

			b, err := node.NewBee(c.config.GetString(optionNameP2PAddr), signerConfig.publicKey, signerConfig.signer, networkID, logger, signerConfig.libp2pPrivateKey, signerConfig.pssPrivateKey, &node.Options{
				DataDir:                    c.config.GetString(optionNameDataDir),
				CacheCapacity:              c.config.GetUint64(optionNameCacheCapacity),
				DBOpenFilesLimit:           c.config.GetUint64(optionNameDBOpenFilesLimit),
				DBBlockCacheCapacity:       c.config.GetUint64(optionNameDBBlockCacheCapacity),
				DBWriteBufferSize:          c.config.GetUint64(optionNameDBWriteBufferSize),
				DBDisableSeeksCompaction:   c.config.GetBool(optionNameDBDisableSeeksCompaction),
				APIAddr:                    c.config.GetString(optionNameAPIAddr),
				DebugAPIAddr:               debugAPIAddr,
				Addr:                       c.config.GetString(optionNameP2PAddr),
				NATAddr:                    c.config.GetString(optionNameNATAddr),
				EnableWS:                   c.config.GetBool(optionNameP2PWSEnable),
				EnableQUIC:                 c.config.GetBool(optionNameP2PQUICEnable),
				WelcomeMessage:             c.config.GetString(optionWelcomeMessage),
				Bootnodes:                  networkConfig.bootNodes,
				CORSAllowedOrigins:         c.config.GetStringSlice(optionCORSAllowedOrigins),
				Standalone:                 c.config.GetBool(optionNameStandalone),
				TracingEnabled:             c.config.GetBool(optionNameTracingEnabled),
				TracingEndpoint:            c.config.GetString(optionNameTracingEndpoint),
				TracingServiceName:         c.config.GetString(optionNameTracingServiceName),
				Logger:                     logger,
				GlobalPinningEnabled:       c.config.GetBool(optionNameGlobalPinningEnabled),
				PaymentThreshold:           c.config.GetString(optionNamePaymentThreshold),
				PaymentTolerance:           c.config.GetString(optionNamePaymentTolerance),
				PaymentEarly:               c.config.GetString(optionNamePaymentEarly),
				ResolverConnectionCfgs:     resolverCfgs,
				GatewayMode:                c.config.GetBool(optionNameGatewayMode),
				BootnodeMode:               bootNode,
				SwapEndpoint:               c.config.GetString(optionNameSwapEndpoint),
				SwapFactoryAddress:         c.config.GetString(optionNameSwapFactoryAddress),
				SwapLegacyFactoryAddresses: c.config.GetStringSlice(optionNameSwapLegacyFactoryAddresses),
				SwapInitialDeposit:         c.config.GetString(optionNameSwapInitialDeposit),
				SwapEnable:                 c.config.GetBool(optionNameSwapEnable),
				FullNodeMode:               fullNode,
				Transaction:                c.config.GetString(optionNameTransactionHash),
				BlockHash:                  c.config.GetString(optionNameBlockHash),
				PostageContractAddress:     c.config.GetString(optionNamePostageContractAddress),
				PriceOracleAddress:         c.config.GetString(optionNamePriceOracleAddress),
				BlockTime:                  networkConfig.blockTime,
				DeployGasPrice:             c.config.GetString(optionNameSwapDeploymentGasPrice),
				WarmupTime:                 c.config.GetDuration(optionWarmUpTime),
				ChainID:                    networkConfig.chainID,
			})
			if err != nil {
				return err
			}

			// Wait for termination or interrupt signals.
			// We want to clean up things at the end.
			interruptChannel := make(chan os.Signal, 1)
			signal.Notify(interruptChannel, syscall.SIGINT, syscall.SIGTERM)

			p := &program{
				start: func() {
					// Block main goroutine until it is interrupted
					sig := <-interruptChannel

					logger.Debugf("received signal: %v", sig)
					logger.Info("shutting down")
				},
				stop: func() {
					// Shutdown
					done := make(chan struct{})
					go func() {
						defer close(done)

						ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
						defer cancel()

						if err := b.Shutdown(ctx); err != nil {
							logger.Errorf("shutdown: %v", err)
						}
					}()

					// If shutdown function is blocking too long,
					// allow process termination by receiving another signal.
					select {
					case sig := <-interruptChannel:
						logger.Debugf("received signal: %v", sig)
					case <-done:
					}
				},
			}

			if isWindowsService {
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
}

func waitForClef(logger logging.Logger, maxRetries uint64, endpoint string) (externalSigner *external.ExternalSigner, err error) {
	for {
		externalSigner, err = external.NewExternalSigner(endpoint)
		if err == nil {
			return externalSigner, nil
		}
		if maxRetries == 0 {
			return nil, err
		}
		maxRetries--
		logger.Warningf("failing to connect to clef signer: %v", err)

		time.Sleep(5 * time.Second)
	}
}

func (c *command) configureSigner(cmd *cobra.Command, logger logging.Logger) (config *signerConfig, err error) {
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
	if p := c.config.GetString(optionNamePassword); p != "" {
		password = p
	} else if pf := c.config.GetString(optionNamePasswordFile); pf != "" {
		b, err := ioutil.ReadFile(pf)
		if err != nil {
			return nil, err
		}
		password = string(bytes.Trim(b, "\n"))
	} else {
		// if libp2p key exists we can assume all required keys exist
		// so prompt for a password to unlock them
		// otherwise prompt for new password with confirmation to create them
		exists, err := keystore.Exists("libp2p")
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

	if c.config.GetBool(optionNameClefSignerEnable) {
		endpoint := c.config.GetString(optionNameClefSignerEndpoint)
		if endpoint == "" {
			endpoint, err = clef.DefaultIpcPath()
			if err != nil {
				return nil, err
			}
		}

		externalSigner, err := waitForClef(logger, 5, endpoint)
		if err != nil {
			return nil, err
		}

		clefRPC, err := rpc.Dial(endpoint)
		if err != nil {
			return nil, err
		}

		wantedAddress := c.config.GetString(optionNameClefSignerEthereumAddress)
		var overlayEthAddress *common.Address = nil
		// if wantedAddress was specified use that, otherwise clef account 0 will be selected.
		if wantedAddress != "" {
			ethAddress := common.HexToAddress(wantedAddress)
			overlayEthAddress = &ethAddress
		}

		signer, err = clef.NewSigner(externalSigner, clefRPC, crypto.Recover, overlayEthAddress)
		if err != nil {
			return nil, err
		}

		publicKey, err = signer.PublicKey()
		if err != nil {
			return nil, err
		}
	} else {
		logger.Warning("clef is not enabled; portability and security of your keys is sub optimal")
		swarmPrivateKey, _, err := keystore.Key("swarm", password)
		if err != nil {
			return nil, fmt.Errorf("swarm key: %w", err)
		}
		signer = crypto.NewDefaultSigner(swarmPrivateKey)
		publicKey = &swarmPrivateKey.PublicKey
	}

	logger.Infof("swarm public key %x", crypto.EncodeSecp256k1PublicKey(publicKey))

	libp2pPrivateKey, created, err := keystore.Key("libp2p", password)
	if err != nil {
		return nil, fmt.Errorf("libp2p key: %w", err)
	}
	if created {
		logger.Debugf("new libp2p key created")
	} else {
		logger.Debugf("using existing libp2p key")
	}

	pssPrivateKey, created, err := keystore.Key("pss", password)
	if err != nil {
		return nil, fmt.Errorf("pss key: %w", err)
	}
	if created {
		logger.Debugf("new pss key created")
	} else {
		logger.Debugf("using existing pss key")
	}

	logger.Infof("pss public key %x", crypto.EncodeSecp256k1PublicKey(&pssPrivateKey.PublicKey))

	// postinst and post scripts inside packaging/{deb,rpm} depend and parse on this log output
	overlayEthAddress, err := signer.EthereumAddress()
	if err != nil {
		return nil, err
	}
	logger.Infof("using ethereum address %x", overlayEthAddress)

	return &signerConfig{
		signer:           signer,
		publicKey:        publicKey,
		libp2pPrivateKey: libp2pPrivateKey,
		pssPrivateKey:    pssPrivateKey,
	}, nil
}

type mainnetConfig struct {
	bootNodes []string
	blockTime uint64
	chainID   int64
}

func getDefaultNetworkConfig(networkID uint64) *mainnetConfig {
	var cfg mainnetConfig
	defaultEthBlockTime := uint64(15 * time.Second)
	switch networkID {
	case 1:
		cfg.bootNodes = []string{"/dnsaddr/mainnet.ethswarm.org"}
		cfg.blockTime = uint64(5 * time.Second)
		cfg.chainID = 100
	case 5: //staging
		cfg.bootNodes = []string{"/dnsaddr/staging.ethswarm.org"}
		cfg.blockTime = defaultEthBlockTime
		cfg.chainID = 5
	default:
		cfg.bootNodes = []string{}
		cfg.blockTime = defaultEthBlockTime
		cfg.chainID = -1 // it will be rejected downstream since it's not valid chain ID
	}

	return &cfg
}
