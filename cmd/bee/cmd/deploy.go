// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/node"
	"github.com/ethersphere/bee/pkg/settlement/swap/erc20"
	"github.com/spf13/cobra"
)

const blocktime = 15

func (c *command) initDeployCmd() error {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy and fund the chequebook contract",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if (len(args)) > 0 {
				return cmd.Help()
			}

			v := strings.ToLower(c.config.GetString(optionNameVerbosity))
			logger, err := newLogger(cmd, v)
			if err != nil {
				return fmt.Errorf("new logger: %w", err)
			}

			dataDir := c.config.GetString(optionNameDataDir)
			factoryAddress := c.config.GetString(optionNameSwapFactoryAddress)
			swapInitialDeposit := c.config.GetString(optionNameSwapInitialDeposit)
			swapEndpoint := c.config.GetString(optionNameSwapEndpoint)
			deployGasPrice := c.config.GetString(optionNameSwapDeploymentGasPrice)
			networkID := c.config.GetUint64(optionNameNetworkID)

			stateStore, err := node.InitStateStore(logger, dataDir)
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
				swapEndpoint,
				0,
				signer,
				blocktime,
				true,
			)
			if err != nil {
				return err
			}
			defer swapBackend.Close()
			defer transactionMonitor.Close()

			chequebookFactory, err := node.InitChequebookFactory(
				logger,
				swapBackend,
				chainID,
				transactionService,
				factoryAddress,
				nil,
			)
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
				deployGasPrice,
				erc20Service,
			)
			if err != nil {
				return err
			}

			optionTrxHash := c.config.GetString(optionNameTransactionHash)
			optionBlockHash := c.config.GetString(optionNameBlockHash)

			txHash, err := node.GetTxHash(stateStore, logger, optionTrxHash)
			if err != nil {
				return fmt.Errorf("invalid transaction hash: %w", err)
			}

			blockTime := time.Duration(c.config.GetUint64(optionNameBlockTime)) * time.Second

			blockHash, err := node.GetTxNextBlock(ctx, logger, swapBackend, transactionMonitor, blockTime, txHash, optionBlockHash)
			if err != nil {
				return err
			}

			pubKey, err := signer.PublicKey()
			if err != nil {
				return err
			}

			swarmAddress, err := crypto.NewOverlayAddress(*pubKey, networkID, blockHash)
			if err != nil {
				return err
			}

			err = node.CheckOverlayWithStore(swarmAddress, stateStore)

			return err
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return c.config.BindPFlags(cmd.Flags())
		},
	}

	c.setAllFlags(cmd)
	c.root.AddCommand(cmd)

	return nil
}
