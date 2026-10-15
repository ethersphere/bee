// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import (
	"io"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/node"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type (
	Command        = command
	Option         = option
	PasswordReader = passwordReader
)

// SubCommandForTest returns the registered subcommand with the given name.
func (c *Command) SubCommandForTest(name string) *cobra.Command {
	for _, sc := range c.root.Commands() {
		if sc.Name() == name {
			return sc
		}
	}
	return nil
}

// CheckUnknownParamsForTest exposes the unknown-parameter validation.
func (c *Command) CheckUnknownParamsForTest(cmd *cobra.Command) error {
	return c.CheckUnknownParams(cmd, nil)
}

var (
	NewCommand = newCommand

	// avoid unused lint errors until the functions are used
	_ = WithCfgFile
	_ = WithInput
	_ = WithErrorOutput
	_ = WithPasswordReader
)

func WithCfgFile(f string) func(c *Command) {
	return func(c *Command) {
		c.cfgFile = f
	}
}

func WithHomeDir(dir string) func(c *Command) {
	return func(c *Command) {
		c.homeDir = dir
	}
}

func WithArgs(a ...string) func(c *Command) {
	return func(c *Command) {
		c.root.SetArgs(a)
	}
}

func WithInput(r io.Reader) func(c *Command) {
	return func(c *Command) {
		c.root.SetIn(r)
	}
}

func WithOutput(w io.Writer) func(c *Command) {
	return func(c *Command) {
		c.root.SetOut(w)
	}
}

func WithErrorOutput(w io.Writer) func(c *Command) {
	return func(c *Command) {
		c.root.SetErr(w)
	}
}

func WithPasswordReader(r PasswordReader) func(c *Command) {
	return func(c *Command) {
		c.passwordReader = r
	}
}

// SetCfgFileForTest sets the config file path after command construction. It
// must be used instead of WithCfgFile because the global "config" flag's
// StringVar default resets c.cfgFile during flag registration.
func (c *Command) SetCfgFileForTest(f string) {
	c.cfgFile = f
}

// Config keys read by node-mode resolution.
const (
	OptionNameNodeMode                = optionNameNodeMode
	OptionNameFullNode                = optionNameFullNode
	OptionNameBootnodeMode            = optionNameBootnodeMode
	OptionNameSwapEnable              = optionNameSwapEnable
	OptionNameChequebookEnable        = optionNameChequebookEnable
	OptionNameStorageIncentivesEnable = optionNameStorageIncentivesEnable
	ConfigKeyBlockchainRpcEndpoint    = configKeyBlockchainRpcEndpoint
)

// ResolveNodeModeForTest runs node-mode resolution against cfg, leaving the
// resolved option values in cfg.
func ResolveNodeModeForTest(cfg *viper.Viper) (node.NodeMode, error) {
	c := &command{config: cfg, logger: log.Noop}
	return c.resolveNodeMode(c.logger)
}

// BindStartFlagsForTest mirrors the start command's PreRunE: it binds cfg to
// the command flags, then maps the flat blockchain-rpc-* flags onto their
// nested config keys.
func BindStartFlagsForTest(cfg *viper.Viper, startCmd *cobra.Command) error {
	if err := cfg.BindPFlags(startCmd.Flags()); err != nil {
		return err
	}
	c := &command{config: cfg}
	c.bindBlockchainRpcConfig(startCmd)
	return nil
}
