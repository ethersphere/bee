// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const endpointGraphModule = "github.com/ethersphere/bee-diet/endpoint-graph"

// resolveToolDir returns the endpoint-graph project root regardless of cwd.
func resolveToolDir() string {
	if wd, err := os.Getwd(); err == nil {
		if dir := findEndpointGraphRoot(wd); dir != "" {
			return dir
		}
		sibling := filepath.Join(wd, "endpoint-graph")
		if isEndpointGraphRoot(sibling) {
			return sibling
		}
	}

	if exe, err := os.Executable(); err == nil {
		dir, err := filepath.EvalSymlinks(filepath.Dir(exe))
		if err == nil {
			if isEndpointGraphRoot(dir) {
				return dir
			}
		}
	}

	return "."
}

func findEndpointGraphRoot(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		if isEndpointGraphRoot(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func isEndpointGraphRoot(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), endpointGraphModule)
}

func defaultBeeRoot(toolDir string) string {
	return filepath.Clean(filepath.Join(toolDir, "..", "..", "..", "code", "bee"))
}

func defaultGraphsDir(toolDir string) string {
	return filepath.Join(toolDir, "graphs")
}

func resolvePath(base, path string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	if base == "" {
		return path
	}
	return filepath.Join(base, path)
}

// resolveBeeRoot normalizes -bee (absolute or relative) and validates the bee module.
func resolveBeeRoot(flagValue, toolDir string) (string, error) {
	root := strings.TrimSpace(flagValue)
	if root == "" {
		root = defaultBeeRoot(toolDir)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("bee path: %w", err)
	}
	if link, err := filepath.EvalSymlinks(abs); err == nil {
		abs = link
	}
	if err := validateBeeRoot(abs); err != nil {
		return "", err
	}
	return abs, nil
}

func validateBeeRoot(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("bee repo not found at %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("bee path is not a directory: %s", dir)
	}
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return fmt.Errorf("bee repo not found at %s: %w", dir, err)
	}
	if !strings.Contains(string(data), beeModule) {
		return fmt.Errorf("not a bee module at %s (expected %s in go.mod)", dir, beeModule)
	}
	return nil
}

// resolveOutputDir normalizes -out (absolute or relative), applies defaultDir when empty, and creates it.
func resolveOutputDir(flagValue, defaultDir string, stdoutMode bool) (string, error) {
	if stdoutMode || flagValue == "-" {
		return "", nil
	}
	out := strings.TrimSpace(flagValue)
	if out == "" {
		out = defaultDir
	}
	abs, err := filepath.Abs(out)
	if err != nil {
		return "", fmt.Errorf("output path: %w", err)
	}
	if err := ensureDir(abs); err != nil {
		return "", fmt.Errorf("create output dir %s: %w", abs, err)
	}
	return abs, nil
}

// applyPathPositionals fills -bee and -out from trailing positional args (after flag.Parse):
//
//	endpoint-graph -all /abs/path/to/bee
//	endpoint-graph -all /abs/path/to/bee /abs/path/to/graphs
func applyPathPositionals(args []string, beeRoot, outDir *string) {
	if len(args) == 0 {
		return
	}
	if *beeRoot == "" {
		*beeRoot = args[0]
		args = args[1:]
	}
	if *outDir == "" && len(args) > 0 {
		*outDir = args[0]
	}
}
