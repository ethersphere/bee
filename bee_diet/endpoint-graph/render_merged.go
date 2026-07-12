// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"strconv"
	"strings"
)

func renderMergedDOT(mg *MergedGraph) string {
	var b strings.Builder
	fmt.Fprintf(&b, "digraph merged {\n")
	fmt.Fprintf(&b, "  graph [rankdir=LR, bgcolor=white, compound=true];\n")
	fmt.Fprintf(&b, "  node [fontname=Helvetica, fontsize=10];\n")
	fmt.Fprintf(&b, "  edge [fontname=Helvetica, fontsize=8];\n")

	color := map[GraphLayer]string{
		LayerRoute: "lightyellow", LayerCall: "wheat", LayerWiring: "lightblue",
	}

	for _, n := range mg.Nodes {
		col := color[n.Layer]
		if col == "" {
			col = "white"
		}
		label := n.Label
		if n.EndpointCount > 1 {
			label += fmt.Sprintf("\n(%d endpoints)", n.EndpointCount)
		}
		fmt.Fprintf(&b, "  %s [shape=%s, label=%q, fillcolor=%s, style=filled, tooltip=%q];\n",
			dotID(n.ID), dotShape(n.Type), label, col, mergedNodeTooltip(n))
	}

	for _, e := range mg.Edges {
		label := e.Kind
		if e.Branch != "" {
			label += " / " + e.Branch
		}
		if e.EndpointCount > 1 {
			label += fmt.Sprintf(" (%d)", e.EndpointCount)
		}
		tooltip := strings.Join(e.Endpoints, "\n")
		fmt.Fprintf(&b, "  %s -> %s [label=%q, tooltip=%q];\n",
			dotID(e.From), dotID(e.To), label, tooltip)
	}

	fmt.Fprintf(&b, "}\n")
	return b.String()
}

func mergedNodeTooltip(n MergedNode) string {
	if len(n.Endpoints) == 0 {
		return n.ID
	}
	if len(n.Endpoints) <= 8 {
		return strings.Join(n.Endpoints, "\n")
	}
	return strings.Join(n.Endpoints[:8], "\n") + "\n… +" + strconv.Itoa(len(n.Endpoints)-8)
}
