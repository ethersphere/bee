// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/ethersphere/bee/pkg/crypto"
)

func (c *command) initKeysCmd() (err error) {
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "Keys management",
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
			fmt.Printf("swarm public key: 0x%x\n", crypto.EncodeSecp256k1PublicKey(signerConfig.publicKey))

			fmt.Printf("pss public key: 0x%x\n", crypto.EncodeSecp256k1PublicKey(&signerConfig.pssPrivateKey.PublicKey))
			fmt.Printf("pss private key: 0x%x\n", crypto.EncodeSecp256k1PrivateKey(signerConfig.pssPrivateKey))

			fmt.Printf("p2p public key: 0x%x\n", crypto.EncodeSecp256k1PublicKey(&signerConfig.libp2pPrivateKey.PublicKey))
			fmt.Printf("p2p private key: 0x%x\n", crypto.EncodeSecp256k1PrivateKey(signerConfig.libp2pPrivateKey))

			var ethAddr, _ = signerConfig.signer.EthereumAddress()
			fmt.Printf("eth address: 0x%x\n", ethAddr)
			fmt.Printf("eth address private key: 0x%x\n", crypto.EncodeSecp256k1PrivateKey(signerConfig.swarmPrivateKey))

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
