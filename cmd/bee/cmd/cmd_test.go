// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethersphere/bee/v2/cmd/bee/cmd"
)

var homeDir string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "bee-cmd-")
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

func TestLogCallerConfigAccepted(t *testing.T) {
	t.Parallel()

	cfgFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfgFile, []byte("log-caller: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := newCommand(t)
	c.SetCfgFileForTest(cfgFile)

	for _, name := range []string{"start", "init"} {
		sub := c.SubCommandForTest(name)
		if sub == nil {
			t.Fatalf("%s subcommand not found", name)
		}
		if err := c.CheckUnknownParamsForTest(sub); err != nil {
			t.Fatalf("%s rejected log-caller: %v", name, err)
		}
		if sub.Flags().Lookup("log-caller") == nil {
			t.Fatalf("%s is missing the log-caller flag", name)
		}
	}
}
