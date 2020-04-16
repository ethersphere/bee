// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/ethersphere/bee/cmd/bee/cmd"
)

var homeDir string

func TestMain(m *testing.M) {
	dir, err := ioutil.TempDir("", "bee-cmd-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	homeDir = dir

	code := m.Run()
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}

func newCommand(t *testing.T, opts ...cmd.Option) (c *cmd.Command) {
	t.Helper()

	c, err := cmd.NewCommand(append([]cmd.Option{cmd.WithHomeDir(homeDir)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
