// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"github.com/ethersphere/bee/v2/pkg/node"
	"github.com/ethersphere/bee/v2/pkg/settlement/swap/erc20"
	"github.com/spf13/cobra"
)

const blocktime = 15

func (c *command) initDeployCmd() error {
	cmd := &cobra.Command{
		Use:               "deploy",
		Short:             "Deploy and fund the chequebook contract",
		PersistentPreRunE: c.CheckUnknownParams,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if (len(args)) > 0 {
				return cmd.Help()
			}

			logger := c.logger

			dataDir := c.config.GetString(optionNameDataDir)
			factoryAddress := c.config.GetString(optionNameSwapFactoryAddress)
			swapInitialDeposit := c.config.GetString(optionNameSwapInitialDeposit)
			stateStore, _, err := node.InitStateStore(logger, dataDir, 1000)
			if err != nil {
				return err
			}

			defer stateStore.Close()

			signerConfig, err := c.configureSigner(cmd, logger)
			if err != nil {
				return err
			}
			signer := signerConfig.signer

			ctx := cmd.Context()

			swapBackend, overlayEthAddress, chainID, transactionMonitor, transactionService, err := node.InitChain(
				ctx,
				logger,
				stateStore,
				0,
				signer,
				blocktime,
				true,
				c.config.GetUint64(optionNameMinimumGasTipCap),
				c.config.GetUint64(optionNameGasLimitFallback),
				node.BlockchainRPCConfig{
					Endpoint:    c.config.GetString(configKeyBlockchainRpcEndpoint),
					DialTimeout: c.config.GetDuration(configKeyBlockchainRpcDialTimeout),
					TLSTimeout:  c.config.GetDuration(configKeyBlockchainRpcTLSTimeout),
					IdleTimeout: c.config.GetDuration(configKeyBlockchainRpcIdleTimeout),
					Keepalive:   c.config.GetDuration(configKeyBlockchainRpcKeepalive),
				},
				0,
			)
			if err != nil {
				return err
			}
			defer swapBackend.Close()
			defer transactionMonitor.Close()

			chequebookFactory, err := node.InitChequebookFactory(logger, swapBackend, chainID, transactionService, factoryAddress)
			if err != nil {
				return err
			}

			erc20Address, err := chequebookFactory.ERC20Address(ctx)
			if err != nil {
				return err
			}

			erc20Service := erc20.New(transactionService, erc20Address)

			_, err = node.InitChequebookService(
				ctx,
				logger,
				stateStore,
				signer,
				chainID,
				swapBackend,
				overlayEthAddress,
				transactionService,
				chequebookFactory,
				swapInitialDeposit,
				erc20Service,
			)
			if err != nil {
				return err
			}

			return err
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if err := c.preRun(cmd); err != nil {
				return err
			}
			c.bindBlockchainRpcConfig(cmd)
			return nil
		},
	}

	c.setAllFlags(cmd)
	c.root.AddCommand(cmd)

	return nil
}
