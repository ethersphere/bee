// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/node"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func (c *command) initDeployCmd() error {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy and fund the chequebook contract",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if (len(args)) > 0 {
				return cmd.Help()
			}

			var logger logging.Logger
			switch v := strings.ToLower(c.config.GetString(optionNameVerbosity)); v {
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
				return fmt.Errorf("unknown verbosity level %q", v)
			}

			var keyDir string
			if c.config.GetString(optionNameKeyDir) == ""{
				keyDir = c.config.GetString(optionNameDataDir)
			} else{
				keyDir = c.config.GetString(optionNameKeyDir)

			}
			factoryAddress := c.config.GetString(optionNameSwapFactoryAddress)
			swapInitialDeposit := c.config.GetString(optionNameSwapInitialDeposit)
			swapEndpoint := c.config.GetString(optionNameSwapEndpoint)

			stateStore, err := node.InitStateStore(logger, keyDir)
			if err != nil {
				return err
			}

			defer stateStore.Close()

			signerConfig, err := c.configureSigner(cmd, logger)
			if err != nil {
				return err
			}
			signer := signerConfig.signer

			err = node.CheckOverlayWithStore(signerConfig.address, stateStore)
			if err != nil {
				return err
			}

			ctx := cmd.Context()

			swapBackend, overlayEthAddress, chainID, transactionService, err := node.InitChain(
				ctx,
				logger,
				stateStore,
				swapEndpoint,
				signer,
			)
			if err != nil {
				return err
			}
			defer swapBackend.Close()

			chequebookFactory, err := node.InitChequebookFactory(
				logger,
				swapBackend,
				chainID,
				transactionService,
				factoryAddress,
			)
			if err != nil {
				return err
			}

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
			)

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
