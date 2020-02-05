// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd

import "io"

type (
	Command        = command
	Option         = option
	PasswordReader = passwordReader
)

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
