// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func runMergeCLI(args []string) {
	fs := flag.NewFlagSet("merge", flag.ExitOnError)
	toolDir := resolveToolDir()
	inDir := fs.String("in", defaultGraphsDir(toolDir), "directory with per-endpoint graph folders")
	outPath := fs.String("out", filepath.Join(defaultGraphsDir(toolDir), "merged.json"), "output merged graph JSON")
	renderSVGOut := fs.String("svg", "", "optional: also render SVG (requires graphviz dot)")
	var mountFilters mountFlag
	fs.Var(&mountFilters, "mount", "include only endpoints from mount (repeatable; mountAPI, mountBusinessDebug, mountTechnicalDebug)")
	collapsePackages := fs.Bool("compress-packages", true, "collapse package nodes by package path (off when any source graph uses -direct-calls or detailed call nodes)")
	_ = fs.Parse(args)

	absIn, err := filepath.Abs(*inDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "in path: %v\n", err)
		os.Exit(1)
	}

	graphs, err := loadEndpointGraphs(absIn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load graphs: %v\n", err)
		os.Exit(1)
	}
	graphs = filterGraphsByMount(graphs, mountFilters)
	if len(graphs) == 0 {
		if len(mountFilters) > 0 {
			fmt.Fprintf(os.Stderr, "no graph.json matched mount filter %v under %s/*/\n", []string(mountFilters), absIn)
		} else {
			fmt.Fprintf(os.Stderr, "no graph.json files under %s/*/\n", absIn)
		}
		os.Exit(1)
	}
	if len(mountFilters) > 0 {
		fmt.Fprintf(os.Stderr, "mount filter %v → %d endpoint graphs\n", []string(mountFilters), len(graphs))
	}

	merged, err := mergeGraphs(absIn, graphs, *collapsePackages)
	if err != nil {
		fmt.Fprintf(os.Stderr, "merge: %v\n", err)
		os.Exit(1)
	}

	out := *outPath
	if !filepath.IsAbs(out) {
		out, err = filepath.Abs(out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "out path: %v\n", err)
			os.Exit(1)
		}
	}
	if err := ensureDir(filepath.Dir(out)); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}
	if err := writeMergedGraph(out, merged); err != nil {
		fmt.Fprintf(os.Stderr, "write merged: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "merged %d endpoints → %d nodes, %d edges, %d packages\n",
		merged.Stats.EndpointCount, merged.Stats.NodeCount, merged.Stats.EdgeCount, merged.Stats.PackageCount)
	fmt.Fprintf(os.Stderr, "wrote %s\n", out)

	if *renderSVGOut != "" {
		dot := renderMergedDOT(merged)
		if err := renderSVGFromDOT([]byte(dot), *renderSVGOut); err != nil {
			fmt.Fprintf(os.Stderr, "render svg: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "wrote %s\n", *renderSVGOut)
	}
}

func runRenderMergedCLI(args []string) {
	fs := flag.NewFlagSet("render-merged", flag.ExitOnError)
	toolDir := resolveToolDir()
	inPath := fs.String("in", filepath.Join(defaultGraphsDir(toolDir), "merged.json"), "merged graph JSON")
	outSVG := fs.String("svg", "", "render SVG via graphviz")
	_ = fs.Parse(args)

	data, err := os.ReadFile(*inPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read: %v\n", err)
		os.Exit(1)
	}
	var mg MergedGraph
	if err := json.Unmarshal(data, &mg); err != nil {
		fmt.Fprintf(os.Stderr, "parse: %v\n", err)
		os.Exit(1)
	}

	if *outSVG == "" {
		fmt.Fprintln(os.Stderr, "specify -svg")
		os.Exit(2)
	}
	dot := renderMergedDOT(&mg)
	if err := renderSVGFromDOT([]byte(dot), *outSVG); err != nil {
		fmt.Fprintf(os.Stderr, "render svg: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", *outSVG)
}
