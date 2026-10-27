// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cmd_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/rangesync"
)

// TestRangesyncFetchRateFlag (T6.5 part 2): the start command has
// --rangesync-fetch-rate, an integer that defaults to rangesync's own
// DefaultFetchRate, and takes it from a config file too.
func TestRangesyncFetchRateFlag(t *testing.T) {
	t.Parallel()

	const name = "rangesync-fetch-rate"
	start := newCommand(t).SubCommandForTest("start")
	if start == nil {
		t.Fatal("start subcommand not found")
	}
	f := start.Flags().Lookup(name)
	if f == nil {
		t.Fatalf("no --%s flag", name)
	}
	if want := strconv.Itoa(rangesync.DefaultFetchRate); f.DefValue != want {
		t.Fatalf("default %q, want %q", f.DefValue, want)
	}
	if got, err := start.Flags().GetInt(name); err != nil || got != rangesync.DefaultFetchRate {
		t.Fatalf("unset: got %d (%v), want %d", got, err, rangesync.DefaultFetchRate)
	}
	if err := start.Flags().Set(name, "250"); err != nil {
		t.Fatal(err)
	}
	if got, err := start.Flags().GetInt(name); err != nil || got != 250 {
		t.Fatalf("set: got %d (%v), want 250", got, err)
	}
	if err := start.Flags().Set(name, "fast"); err == nil {
		t.Fatal("a value that is not an integer parsed")
	}

	cfgFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfgFile, []byte(name+": 250\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := newCommand(t)
	c.SetCfgFileForTest(cfgFile)
	if err := c.CheckUnknownParamsForTest(c.SubCommandForTest("start")); err != nil {
		t.Fatalf("config key %s: %v", name, err)
	}
}
