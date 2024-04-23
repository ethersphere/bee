// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v2"

	"github.com/spf13/cobra"
)

func (c *command) initConfigurateOptionsCmd() (err error) {

	cmd := &cobra.Command{
		Use:   "printconfig",
		Short: "Print default or provided configuration in yaml format",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			if len(args) > 0 {
				return cmd.Help()
			}

			d := c.config.AllSettings()

			var keys []string
			for k := range d {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			for _, k := range keys {
				v := d[k]
				ym, err := yaml.Marshal(map[any]any{k: v})
				if err != nil {
					return err
				}
				f := cmd.Flag(k)
				if f == nil {
					return fmt.Errorf("%s is not a valid flag", k)
				}
				cmd.Println("#", f.Usage)

				cmd.Print(string(ym))
			}
			cmd.Println()

			return nil
		},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return c.config.BindPFlags(cmd.Flags())
		},
	}

	c.setAllFlags(cmd)

	c.root.AddCommand(cmd)

	return nil
}
