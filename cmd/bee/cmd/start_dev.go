// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"context"
	"fmt"

	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ethersphere/bee"
	"github.com/ethersphere/bee/pkg/node"
	"github.com/ethersphere/bee/pkg/resolver/multiresolver"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/kardianos/service"
	"github.com/spf13/cobra"
)

func (c *command) initStartDevCmd() (err error) {

	cmd := &cobra.Command{
		Use:   "start-dev",
		Short: "Start a Swarm node in development mode",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if len(args) > 0 {
				return cmd.Help()
			}

			v := strings.ToLower(c.config.GetString(optionNameVerbosity))
			logger, err := newLogger(cmd, v)
			if err != nil {
				return fmt.Errorf("new logger: %v", err)
			}

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
			fmt.Println()
			fmt.Println("Starting in development mode")
			fmt.Println()

			debugAPIAddr := c.config.GetString(optionNameDebugAPIAddr)
			if !c.config.GetBool(optionNameDebugAPIEnable) {
				debugAPIAddr = ""
			}

			signerConfig, err := c.configureSigner(cmd, logger)
			if err != nil {
				return err
			}

			logger.Infof("version: %v", bee.Version)

			tracingEndpoint := c.config.GetString(optionNameTracingEndpoint)

			if c.config.IsSet(optionNameTracingHost) && c.config.IsSet(optionNameTracingPort) {
				tracingEndpoint = strings.Join([]string{c.config.GetString(optionNameTracingHost), c.config.GetString(optionNameTracingPort)}, ":")
			}

			b, err := node.NewDevBee(c.config.GetString(optionNameP2PAddr), signerConfig.publicKey, signerConfig.signer, logger, signerConfig.libp2pPrivateKey, signerConfig.pssPrivateKey, &node.Options{
				APIAddr:                c.config.GetString(optionNameAPIAddr),
				DebugAPIAddr:           debugAPIAddr,
				Addr:                   c.config.GetString(optionNameP2PAddr),
				NATAddr:                c.config.GetString(optionNameNATAddr),
				EnableWS:               c.config.GetBool(optionNameP2PWSEnable),
				EnableQUIC:             c.config.GetBool(optionNameP2PQUICEnable),
				WelcomeMessage:         c.config.GetString(optionWelcomeMessage),
				CORSAllowedOrigins:     c.config.GetStringSlice(optionCORSAllowedOrigins),
				TracingEnabled:         c.config.GetBool(optionNameTracingEnabled),
				TracingEndpoint:        tracingEndpoint,
				TracingServiceName:     c.config.GetString(optionNameTracingServiceName),
				Logger:                 logger,
				GlobalPinningEnabled:   c.config.GetBool(optionNameGlobalPinningEnabled),
				ResolverConnectionCfgs: resolverCfgs,
				Transaction:            c.config.GetString(optionNameTransactionHash),
				RetrievalCaching:       c.config.GetBool(optionNameRetrievalCaching),
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

	cmd.Flags().String(optionNameDataDir, "", "data directory")
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
	cmd.Flags().StringSlice(optionNameBootnodes, []string{"/dnsaddr/testnet.ethswarm.org"}, "initial nodes to connect to")
	cmd.Flags().Bool(optionNameDebugAPIEnable, false, "enable debug HTTP API")
	cmd.Flags().String(optionNameDebugAPIAddr, ":1635", "debug HTTP API listen address")
	cmd.Flags().Uint64(optionNameNetworkID, 10, "ID of the Swarm network")
	cmd.Flags().StringSlice(optionCORSAllowedOrigins, []string{}, "origins with CORS headers enabled")
	cmd.Flags().Bool(optionNameTracingEnabled, false, "enable tracing")
	cmd.Flags().String(optionNameTracingEndpoint, "127.0.0.1:6831", "endpoint to send tracing data")
	cmd.Flags().String(optionNameTracingHost, "", "host to send tracing data")
	cmd.Flags().String(optionNameTracingPort, "", "port to send tracing data")
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
	cmd.Flags().String(optionNameBlockHash, "", "block hash of the block whose parent is the block that contains the transaction hash")
	cmd.Flags().Uint64(optionNameBlockTime, 15, "chain block time")
	cmd.Flags().String(optionNameSwapDeploymentGasPrice, "", "gas price in wei to use for deployment and funding")
	cmd.Flags().Duration(optionWarmUpTime, time.Minute*20, "time to warmup the node before pull/push protocols can be kicked off.")
	cmd.Flags().Bool(optionNameMainNet, false, "triggers connect to main net bootnodes.")
	cmd.Flags().Bool(optionNameRetrievalCaching, true, "enable forwarded content caching")

	c.root.AddCommand(cmd)
	return nil
}
