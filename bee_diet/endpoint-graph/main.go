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
	"strings"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "merge":
			runMergeCLI(os.Args[2:])
			return
		case "render-merged":
			runRenderMergedCLI(os.Args[2:])
			return
		case "export-yed":
			runExportYedCLI(os.Args[2:])
			return
		case "entry":
			runProgramGraphCLI(os.Args[2:])
			return
		case "node":
			runNodeGraphCLI(os.Args[2:])
			return
		}
	}
	runAnalyzeCLI()
}

func runAnalyzeCLI() {
	beeRoot := flag.String("bee", "", "absolute or relative path to bee repo (default: ../../../code/bee from tool dir)")
	routerPath := flag.String("router", "", "path to router.go")
	method := flag.String("method", "", "filter by HTTP method")
	path := flag.String("path", "", "filter by path, e.g. /accounting")
	handler := flag.String("handler", "", "analyze by handler name")
	all := flag.Bool("all", false, "analyze all endpoints")
	format := flag.String("format", "text", "stdout output: text, json")
	outDir := flag.String("out", "", "output directory for graphs (default: <tool>/graphs; created if missing)")
	stdout := flag.Bool("stdout", false, "write to stdout")
	svg := flag.Bool("svg", true, "render .svg via graphviz (default with .json)")
	writeText := flag.Bool("text-out", false, "also write .txt tree files")
	writeIndex := flag.Bool("index", false, "also write index.json with all graphs")
	writeShared := flag.Bool("shared", false, "also write shared.json capability fan-in")
	writeIndexMDFlag := flag.Bool("index-md", false, "also write index.md summary table")
	maxDepth := flag.Int("depth", 0, "max SSA call depth (0 = unlimited)")
	directCalls := flag.Bool("direct-calls", true, "method-level capabilities and direct static calls (default)")
	compressPackages := flag.Bool("compress-packages", false, "collapse subsystem calls to one package node per caller (disabled with -direct-calls)")
	flag.Parse()

	toolDir := resolveToolDir()
	applyPathPositionals(flag.Args(), beeRoot, outDir)

	root, err := resolveBeeRoot(*beeRoot, toolDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bee: %v\n", err)
		os.Exit(1)
	}
	rtr := *routerPath
	if rtr == "" {
		rtr = filepath.Join(root, "pkg", "api", "router.go")
	}

	routerData, err := os.ReadFile(rtr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read router: %v\n", err)
		os.Exit(1)
	}
	endpoints, err := ParseRouterAST(rtr, routerData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse router: %v\n", err)
		os.Exit(1)
	}

	if *handler != "" {
		endpoints = []Endpoint{{Handler: *handler, Method: *method, Path: *path, Mount: "manual"}}
	} else if !*all {
		if *path == "" && *method == "" {
			fmt.Fprintln(os.Stderr, "specify -method and -path, -handler, or -all")
			flag.Usage()
			os.Exit(2)
		}
		endpoints = filterEndpoints(endpoints, *method, *path)
	}
	if len(endpoints) == 0 {
		fmt.Fprintln(os.Stderr, "no endpoints matched")
		os.Exit(1)
	}

	fields, err := loadServiceFields(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "service fields: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "loading SSA for %s ...\n", root)
	prog, err := loadSSA(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ssa: %v\n", err)
		os.Exit(1)
	}

	stdoutMode := *stdout || *outDir == "-"
	outputDir, err := resolveOutputDir(*outDir, defaultGraphsDir(toolDir), stdoutMode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	var graphs []*EndpointGraph
	svgOK := 0
	usedDirs := map[string]struct{}{}
	compress := *compressPackages && !*directCalls
	for _, ep := range endpoints {
		g, err := analyzeEndpointGraph(prog, ep, fields, *maxDepth, *directCalls, compress)
		if err != nil {
			fmt.Fprintf(os.Stderr, "analyze %s %s: %v\n", ep.Method, ep.Path, err)
			os.Exit(1)
		}
		graphs = append(graphs, g)

		if stdoutMode && len(endpoints) == 1 {
			if err := emitStdout(g, *format); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
			continue
		}
		if !stdoutMode {
			dirName := uniqueEndpointDirName(ep, usedDirs)
			epDir := filepath.Join(outputDir, dirName)
			if err := ensureDir(epDir); err != nil {
				fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", epDir, err)
				os.Exit(1)
			}
			opts := graphWriteOpts{SVG: *svg, TXT: *writeText}
			svgName, err := writeGraphFiles(epDir, graphFileBase, g, opts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "write %s: %v\n", dirName, err)
				os.Exit(1)
			}
			if svgName != "" {
				svgOK++
			}
		}
	}

	if !stdoutMode {
		if *writeIndex {
			if err := writeJSON(filepath.Join(outputDir, "index.json"), graphs); err != nil {
				fmt.Fprintf(os.Stderr, "index: %v\n", err)
				os.Exit(1)
			}
		}
		if *writeShared {
			shared := buildSharedIndex(graphs)
			if err := writeJSON(filepath.Join(outputDir, "shared.json"), shared); err != nil {
				fmt.Fprintf(os.Stderr, "shared: %v\n", err)
				os.Exit(1)
			}
		}
		if *writeIndexMDFlag {
			if err := writeIndexMD(filepath.Join(outputDir, "index.md"), graphs); err != nil {
				fmt.Fprintf(os.Stderr, "index.md: %v\n", err)
				os.Exit(1)
			}
		}
		fmt.Fprintf(os.Stderr, "wrote %d graphs to %s", len(graphs), outputDir)
		if *svg {
			fmt.Fprintf(os.Stderr, " (%d svg)", svgOK)
		}
		fmt.Fprintln(os.Stderr)
	} else if len(endpoints) > 1 {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(graphs); err != nil {
			fmt.Fprintf(os.Stderr, "json: %v\n", err)
			os.Exit(1)
		}
	}
}

func filterEndpoints(endpoints []Endpoint, method, path string) []Endpoint {
	var out []Endpoint
	for _, ep := range endpoints {
		if method != "" && !strings.EqualFold(ep.Method, method) {
			continue
		}
		if path != "" && ep.Path != path {
			continue
		}
		out = append(out, ep)
	}
	return out
}

func emitStdout(g *EndpointGraph, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(g)
	case "text":
		fmt.Print(renderTextGraph(g))
		return nil
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func uniqueEndpointDirName(ep Endpoint, used map[string]struct{}) string {
	name := EndpointDirName(ep)
	if _, ok := used[name]; ok {
		name = name + "_" + ep.Handler
	}
	used[name] = struct{}{}
	return name
}
