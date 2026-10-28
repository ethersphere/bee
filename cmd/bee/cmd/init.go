// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"github.com/ethersphere/bee/v2/pkg/node"
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

			_, err = c.configureSigner(cmd, c.logger)
			if err != nil {
				return err
			}

			dataDir := c.config.GetString(optionNameDataDir)
			stateStore, _, err := node.InitStateStore(c.logger, dataDir, 1000)
			if err != nil {
				return err
			}

			defer stateStore.Close()

			return nil
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return c.preRun(cmd)
		},
	}

	c.setAllFlags(cmd)
	c.root.AddCommand(cmd)
	return nil
}
