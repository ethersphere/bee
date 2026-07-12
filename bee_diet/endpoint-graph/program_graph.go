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

func runNodeGraphCLI(args []string) {
	prefill := []string{"-entry", "node"}
	runProgramGraphCLI(append(prefill, args...))
}

func runProgramGraphCLI(args []string) {
	fs := flag.NewFlagSet("entry", flag.ExitOnError)
	beeRoot := fs.String("bee", "", "absolute or relative path to bee repo (default: ../../../code/bee from tool dir)")
	entry := fs.String("entry", "node", "entry root: node, pkg/node:NewBee, main, cmd/bee/main.go:main")
	format := fs.String("format", "text", "stdout output: text, json")
	outDir := fs.String("out", "", "output directory (default: ./graphs-node or ./graphs-entry; created if missing)")
	stdout := fs.Bool("stdout", false, "write to stdout")
	svg := fs.Bool("svg", true, "render .svg via graphviz")
	writeText := fs.Bool("text-out", false, "also write .txt tree files")
	maxDepth := fs.Int("depth", 0, "max SSA call depth (0 = unlimited)")
	directCalls := fs.Bool("direct-calls", true, "method-level capabilities and direct static calls")
	compressPackages := fs.Bool("compress-packages", false, "collapse subsystem calls to package nodes")
	_ = fs.Parse(args)

	toolDir := resolveToolDir()
	applyPathPositionals(fs.Args(), beeRoot, outDir)

	root, err := resolveBeeRoot(*beeRoot, toolDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bee: %v\n", err)
		os.Exit(1)
	}
	spec, err := parseEntrySpec(*entry, root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "entry: %v\n", err)
		os.Exit(2)
	}
	fields, err := loadServiceFields(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "service fields: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "loading SSA for %s (entry %s.%s) ...\n", root, spec.PkgRel, spec.FuncName)
	prog, err := loadSSAForEntry(root, spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ssa: %v\n", err)
		os.Exit(1)
	}

	compress := *compressPackages && !*directCalls
	g, err := analyzeProgramGraph(prog, spec, fields, *maxDepth, *directCalls, compress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "analyze entry: %v\n", err)
		os.Exit(1)
	}

	stdoutMode := *stdout || *outDir == "-"
	outputDir, err := resolveOutputDir(*outDir, defaultProgramGraphsDir(toolDir, spec), stdoutMode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	if stdoutMode {
		if err := emitProgramStdout(g, *format); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "nodes=%d edges=%d packages=%d\n", len(g.Nodes), len(g.Edges), len(g.CapabilityPackages))
		return
	}

	opts := graphWriteOpts{SVG: *svg, TXT: *writeText}
	base := programGraphBase(spec)
	svgName, err := writeProgramGraphFiles(outputDir, base, g, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "wrote program graph to %s (%d nodes, %d edges, %d pkg)", outputDir, len(g.Nodes), len(g.Edges), len(g.CapabilityPackages))
	if svgName != "" {
		fmt.Fprintf(os.Stderr, ", svg=%s", svgName)
	}
	fmt.Fprintln(os.Stderr)
}

func defaultProgramGraphsDir(toolDir string, spec entrySpec) string {
	if spec.PkgRel == "pkg/node" && spec.FuncName == "NewBee" {
		return filepath.Join(toolDir, "graphs-node")
	}
	if spec.PkgRel == "cmd/bee" && spec.FuncName == "main" {
		return filepath.Join(toolDir, "graphs-entry")
	}
	return filepath.Join(toolDir, "graphs-program")
}

func programGraphBase(spec entrySpec) string {
	if spec.PkgRel == "pkg/node" && spec.FuncName == "NewBee" {
		return "node"
	}
	if spec.PkgRel == "cmd/bee" && spec.FuncName == "main" {
		return "program"
	}
	return sanitizeScopePart(spec.PkgRel + "." + spec.FuncName)
}

func emitProgramStdout(g *ProgramGraph, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(g)
	case "text":
		fmt.Print(renderTextProgramGraph(g))
		return nil
	default:
		return fmt.Errorf("unsupported format %q", format)
	}
}

func writeProgramGraphFiles(dir, base string, g *ProgramGraph, opts graphWriteOpts) (svgFile string, err error) {
	jsonPath := filepath.Join(dir, base+".json")
	if err := writeJSON(jsonPath, g); err != nil {
		return "", err
	}
	if opts.TXT {
		if err := os.WriteFile(filepath.Join(dir, base+".txt"), []byte(renderTextProgramGraph(g)), 0o644); err != nil {
			return "", err
		}
	}
	if opts.SVG {
		dot := []byte(renderDOTProgramGraph(g))
		svgPath := filepath.Join(dir, base+".svg")
		if err := renderSVGFromDOT(dot, svgPath); err != nil {
			fmt.Fprintf(os.Stderr, "svg skip %s: %v\n", base, err)
			return "", nil
		}
		return base + ".svg", nil
	}
	return "", nil
}

func renderTextProgramGraph(g *ProgramGraph) string {
	return renderTextGraph(programGraphAsEndpoint(g))
}

func programGraphAsEndpoint(g *ProgramGraph) *EndpointGraph {
	return &EndpointGraph{
		Endpoint: Endpoint{
			Mount:   "program",
			Method:  "ENTRY",
			Path:    g.Entry.Package + ":" + g.Entry.Function,
			Handler: g.Entry.Function,
		},
		Nodes:              g.Nodes,
		Edges:              g.Edges,
		CapabilityPackages: g.CapabilityPackages,
		DirectCalls:        g.DirectCalls,
		CompressPackages:   g.CompressPackages,
		EntityProjection:   g.EntityProjection,
	}
}

func renderDOTProgramGraph(g *ProgramGraph) string {
	return renderDOTGraph(programGraphAsEndpoint(g))
}
