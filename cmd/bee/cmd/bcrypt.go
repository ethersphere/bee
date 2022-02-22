// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
)

func (c *command) initHasherCmd() (err error) {
	cmd := &cobra.Command{
		Use:   "bcrypt",
		Short: "Generate or validate a bcrypt hash",
		Long: `Generate or validate a bcrypt hash

Takes a single plain text argument in order to generate a bcrypt hash.
If '--check' flag is provided it will validate the first (plain text) argument against
the second one, which is expected to be a quoted bcrypt hash.`,
		Example: `
$> bee bcrypt super$ecret
$2a$10$eZP5YuhJq2k8DFmj9UJGWOIjDtXu6NcAQMrz7Zj1bgIVBcHA3bU5u

$> bee bcrypt --check super$ecret '$2a$10$eZP5YuhJq2k8DFmj9UJGWOIjDtXu6NcAQMrz7Zj1bgIVBcHA3bU5u'
OK: password hash matches provided plain text`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 || len(args) > 2 {
				return cmd.Help()
			}

			isCheck := c.config.GetBool("check")

			if isCheck {
				if len(args) != 2 {
					fmt.Println("Usage:", "bee bcrypt", "--check", "your-plain-text-password", "'password-hash'")
					return nil
				}

				err := bcrypt.CompareHashAndPassword([]byte(args[1]), []byte(args[0]))
				if err != nil {
					return errors.New("password hash does not match provided plain text")
				}

				fmt.Println("OK: password hash matches provided plain text")
				return nil
			}

			if len(args) != 1 {
				return cmd.Help()
			}

			hashed, err := bcrypt.GenerateFromPassword([]byte(args[0]), bcrypt.DefaultCost)
			if err != nil {
				return errors.New("failed to generate password hash")
			}

			fmt.Print(string(hashed))
			return nil
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return c.config.BindPFlags(cmd.Flags())
		},
	}

	cmd.Flags().Bool("check", false, "validate existing hash")

	c.root.AddCommand(cmd)
	return nil
}
