// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"io/ioutil"
	"os"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/logging"
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

			res, err := c.configureSigner(cmd, logging.New(ioutil.Discard, 0))

			cmd.SetOut(os.Stdout)
			cmd.Printf("0x%x\n", crypto.EncodeSecp256k1PublicKey(res.publicKey))
			cmd.Printf("0x%x\n", crypto.EncodeSecp256k1PublicKey(&res.pssPrivateKey.PublicKey))
			cmd.Printf("0x%x\n", res.overlayEthAddress)

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
