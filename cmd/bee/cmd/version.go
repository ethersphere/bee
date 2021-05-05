// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"os"

	"github.com/ethersphere/bee"

	"github.com/spf13/cobra"
)

func (c *command) initVersionCmd() {
	v := &cobra.Command{
		Use:   "version",
		Short: "Print version number",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println(bee.Version)
		},
	}
	v.SetOut(os.Stdout)
	c.root.AddCommand(v)
}
