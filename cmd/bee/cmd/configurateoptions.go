// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"github.com/spf13/cobra"
)

func (c *command) initConfigurateOptionsCmd() {
	c.root.AddCommand(&cobra.Command{
		Use:   "config",
		Short: "Print configuration options",
		Run: func(cmd *cobra.Command, args []string) {
			d := c.config.AllSettings()
			cmd.Printf("%v\n", d)
		},
	})
}
