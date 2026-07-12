// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveToolDirFromModuleRoot(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if !isEndpointGraphRoot(wd) {
		t.Fatalf("tests must run from endpoint-graph module root, cwd=%s", wd)
	}

	got := resolveToolDir()
	absGot, err := filepath.Abs(got)
	if err != nil {
		t.Fatal(err)
	}
	absWant, err := filepath.Abs(wd)
	if err != nil {
		t.Fatal(err)
	}
	if absGot != absWant {
		t.Fatalf("resolveToolDir()=%q want %q", absGot, absWant)
	}
}

func TestDefaultGraphsDirUnderToolRoot(t *testing.T) {
	t.Parallel()

	toolDir := resolveToolDir()
	got := defaultGraphsDir(toolDir)
	if !strings.HasSuffix(filepath.ToSlash(got), "/endpoint-graph/graphs") &&
		!strings.HasSuffix(filepath.ToSlash(got), "endpoint-graph/graphs") {
		t.Fatalf("defaultGraphsDir=%q", got)
	}
}

func TestDefaultBeeRootFromToolDir(t *testing.T) {
	t.Parallel()

	toolDir := resolveToolDir()
	root := defaultBeeRoot(toolDir)
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("defaultBeeRoot=%q not a go module: %v", root, err)
	}
}

func TestResolveBeeRootAbsolute(t *testing.T) {
	t.Parallel()

	toolDir := resolveToolDir()
	want := defaultBeeRoot(toolDir)
	absWant, err := filepath.Abs(want)
	if err != nil {
		t.Fatal(err)
	}

	got, err := resolveBeeRoot(absWant, toolDir)
	if err != nil {
		t.Fatal(err)
	}
	if got != absWant {
		t.Fatalf("resolveBeeRoot()=%q want %q", got, absWant)
	}
}

func TestResolveOutputDirCreatesDefault(t *testing.T) {
	t.Parallel()

	toolDir := resolveToolDir()
	dir, err := resolveOutputDir("", defaultGraphsDir(toolDir), false)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("output dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("output path is not a directory: %s", dir)
	}
}

func TestApplyPathPositionals(t *testing.T) {
	t.Parallel()

	bee := ""
	out := ""
	applyPathPositionals([]string{"/tmp/bee", "/tmp/graphs"}, &bee, &out)
	if bee != "/tmp/bee" || out != "/tmp/graphs" {
		t.Fatalf("bee=%q out=%q", bee, out)
	}

	bee = "/flag/bee"
	out = ""
	applyPathPositionals([]string{"/pos/out"}, &bee, &out)
	if bee != "/flag/bee" || out != "/pos/out" {
		t.Fatalf("flag bee must win: bee=%q out=%q", bee, out)
	}
}

func TestResolveToolDirFromSiblingBeeDiet(t *testing.T) {
	t.Parallel()

	if !isEndpointGraphRoot(".") {
		t.Skip("not in endpoint-graph module")
	}
	parent, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(parent, "endpoint-graph")
	if !isEndpointGraphRoot(sibling) {
		t.Skip("no endpoint-graph sibling")
	}

	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(parent); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	got, err := filepath.Abs(resolveToolDir())
	if err != nil {
		t.Fatal(err)
	}
	if got != sibling {
		t.Fatalf("from bee_diet resolveToolDir()=%q want %q", got, sibling)
	}
}
