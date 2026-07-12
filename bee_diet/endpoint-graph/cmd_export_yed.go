// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func runExportYedCLI(args []string) {
	fs := flag.NewFlagSet("export-yed", flag.ExitOnError)
	toolDir := resolveToolDir()
	inPath := fs.String("in", filepath.Join(defaultGraphsDir(toolDir), "merged.json"), "input graph.json or merged.json")
	graphmlPath := fs.String("graphml", "", "output .graphml file (default: input path with .graphml extension)")
	stdout := fs.Bool("stdout", false, "write GraphML to stdout")
	essentialColor := fs.Bool("essential-color", false, "fill nodes from essential packages red (#FF6666)")
	_ = fs.Parse(args)

	if *inPath == "" {
		fmt.Fprintln(os.Stderr, "specify -in")
		os.Exit(2)
	}

	absIn, err := filepath.Abs(*inPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "in path: %v\n", err)
		os.Exit(1)
	}

	jsonData, err := os.ReadFile(absIn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read graph: %v\n", err)
		os.Exit(1)
	}

	opts := yedExportOptions{EssentialColor: *essentialColor}
	data, err := renderYedExportFromJSON(jsonData, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render graphml: %v\n", err)
		os.Exit(1)
	}

	g, _ := parseYedExportGraph(jsonData)

	if *stdout {
		os.Stdout.Write(data)
		fmt.Fprintf(os.Stderr, "exported %d nodes, %d edges to stdout\n", len(g.Nodes), len(g.Edges))
		return
	}

	out := *graphmlPath
	if out == "" {
		out = strings.TrimSuffix(absIn, filepath.Ext(absIn)) + ".graphml"
	} else if !filepath.IsAbs(out) {
		out, err = filepath.Abs(out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "graphml path: %v\n", err)
			os.Exit(1)
		}
	}

	if err := os.WriteFile(out, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "exported %d nodes, %d edges → %s\n", len(g.Nodes), len(g.Edges), out)
}
