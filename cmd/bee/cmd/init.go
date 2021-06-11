// Copyright 2020 The Swarm Authors. All rights reserved.
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

func (c *command) initInitCmd() (err error) {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialise a Swarm node",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if len(args) > 0 {
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

			signerConfig, err := c.configureSigner(cmd, logger)
			if err != nil {
				return err
			}

			var keyDir string
			if c.config.GetString(optionNameKeyDir) == ""{
				keyDir = c.config.GetString(optionNameDataDir)
			} else{
				keyDir = c.config.GetString(optionNameKeyDir)

			}
			stateStore, err := node.InitStateStore(logger, keyDir)
			if err != nil {
				return err
			}

			defer stateStore.Close()

			return node.CheckOverlayWithStore(signerConfig.address, stateStore)
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return c.config.BindPFlags(cmd.Flags())
		},
	}

	c.setAllFlags(cmd)
	c.root.AddCommand(cmd)
	return nil
}
