// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func renderTextGraph(g *EndpointGraph) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", g.Endpoint.Method, g.Endpoint.Path)
	fmt.Fprintf(&b, "mount: %s  handler: %s\n", g.Endpoint.Mount, g.Endpoint.Handler)
	if len(g.Endpoint.Middlewares) > 0 {
		fmt.Fprintf(&b, "middlewares: %s\n", strings.Join(g.Endpoint.Middlewares, " → "))
	}
	fmt.Fprintf(&b, "packages (%d): %s\n", len(g.CapabilityPackages), strings.Join(g.CapabilityPackages, ", "))
	fmt.Fprintln(&b)

	children := adjacency(g.Edges)
	roots := rootNodes(g)
	seen := map[string]struct{}{}
	for _, root := range roots {
		renderGraphASCII(&b, g, children, root, "", true, seen, 0)
	}
	return b.String()
}

func rootNodes(g *EndpointGraph) []string {
	hasParent := map[string]struct{}{}
	for _, e := range g.Edges {
		hasParent[e.To] = struct{}{}
	}
	var roots []string
	nodeByID := map[string]GraphNode{}
	for _, n := range g.Nodes {
		nodeByID[n.ID] = n
	}
	for _, n := range g.Nodes {
		if n.Layer == LayerRoute && (n.Type == NodeRoute || n.Type == NodeEntry) {
			roots = append(roots, n.ID)
		}
	}
	if len(roots) == 0 {
		for id := range nodeByID {
			if _, ok := hasParent[id]; !ok {
				roots = append(roots, id)
			}
		}
	}
	sortGraphNodesByID(roots)
	return roots
}

func sortGraphNodesByID(ids []string) {
	sort.Strings(ids)
}

func adjacency(edges []GraphEdge) map[string][]string {
	out := map[string][]string{}
	for _, e := range edges {
		out[e.From] = append(out[e.From], e.To)
	}
	for k := range out {
		seen := map[string]struct{}{}
		var uniq []string
		for _, v := range out[k] {
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			uniq = append(uniq, v)
		}
		out[k] = uniq
	}
	return out
}

func renderGraphASCII(b *strings.Builder, g *EndpointGraph, children map[string][]string, id, prefix string, isLast bool, seen map[string]struct{}, depth int) {
	if depth > 40 {
		return
	}
	if _, dup := seen[id]; dup {
		fmt.Fprintf(b, "%s%s (…)\n", prefix, nodeLabel(g, id))
		return
	}
	seen[id] = struct{}{}

	n := nodeByID(g, id)

	branch := "├── "
	if isLast {
		branch = "└── "
	}
	if prefix == "" {
		branch = ""
	}
	label := nodeLabel(g, id)
	if n.Op != "" {
		label += " [" + n.Op + "]"
	}
	if n.Layer == LayerWiring {
		label += " {wiring}"
	}
	fmt.Fprintf(b, "%s%s%s\n", prefix, branch, label)

	childPrefix := prefix
	if isLast {
		if prefix == "" {
			childPrefix = "    "
		} else {
			childPrefix += "    "
		}
	} else {
		if prefix == "" {
			childPrefix = "│   "
		} else {
			childPrefix += "│   "
		}
	}
	ch := children[id]
	for i, c := range ch {
		renderGraphASCII(b, g, children, c, childPrefix, i == len(ch)-1, seen, depth+1)
	}
}

func nodeByID(g *EndpointGraph, id string) GraphNode {
	for _, n := range g.Nodes {
		if n.ID == id {
			return n
		}
	}
	return GraphNode{ID: id, Label: id}
}

func nodeLabel(g *EndpointGraph, id string) string {
	n := nodeByID(g, id)
	if n.Label != "" {
		return n.Label
	}
	return id
}

func renderDOTGraph(g *EndpointGraph) string {
	var b strings.Builder
	fmt.Fprintf(&b, "digraph endpoint {\n")
	fmt.Fprintf(&b, "  graph [rankdir=LR, bgcolor=white];\n")
	fmt.Fprintf(&b, "  node [fontname=Helvetica, fontsize=10];\n")

	color := map[GraphLayer]string{
		LayerRoute: "lightyellow", LayerCall: "wheat", LayerWiring: "lightblue",
	}

	seen := map[string]struct{}{}
	for _, n := range g.Nodes {
		id := dotID(n.ID)
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		col := color[n.Layer]
		if col == "" {
			col = "white"
		}
		fmt.Fprintf(&b, "  %s [shape=%s, label=%q, fillcolor=%s, style=filled];\n", id, dotShape(n.Type), n.Label, col)
	}

	for _, e := range g.Edges {
		fmt.Fprintf(&b, "  %s -> %s [label=%q];\n", dotID(e.From), dotID(e.To), e.Kind)
	}
	fmt.Fprintf(&b, "}\n")
	return b.String()
}

func dotShape(t NodeType) string {
	switch t {
	case NodeEntry, NodeRoute:
		return "ellipse"
	case NodeFunction, NodeHandler, NodeMiddleware:
		return "box"
	case NodeServiceField:
		return "diamond"
	case NodeCapability, NodeEntity:
		return "hexagon"
	case NodeImplementation:
		return "component"
	case NodeBackend:
		return "cylinder"
	case NodeExternalDeps:
		return "note"
	default:
		return "box"
	}
}

func dotID(s string) string {
	id := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		default:
			return '_'
		}
	}, s)
	if id == "" {
		return "n"
	}
	if id[0] >= '0' && id[0] <= '9' {
		return "n_" + id
	}
	return id
}

func renderSVGFromDOT(dot []byte, svgPath string) error {
	cmd := exec.Command("dot", "-Tsvg", "-o", svgPath)
	cmd.Stdin = bytes.NewReader(dot)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("dot -Tsvg: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

type graphWriteOpts struct {
	SVG bool
	TXT bool
}

func writeGraphFiles(dir, base string, g *EndpointGraph, opts graphWriteOpts) (svgFile string, err error) {
	if err := writeJSON(filepath.Join(dir, base+".json"), g); err != nil {
		return "", err
	}
	if opts.TXT {
		if err := os.WriteFile(filepath.Join(dir, base+".txt"), []byte(renderTextGraph(g)), 0o644); err != nil {
			return "", err
		}
	}
	if !opts.SVG {
		return "", nil
	}
	dot := []byte(renderDOTGraph(g))
	svgPath := filepath.Join(dir, base+".svg")
	if err := renderSVGFromDOT(dot, svgPath); err != nil {
		fmt.Fprintf(os.Stderr, "svg skip %s: %v\n", base, err)
		return "", nil
	}
	return base + ".svg", nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
