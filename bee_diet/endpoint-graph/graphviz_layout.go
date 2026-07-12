// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

const graphvizInchesToPixels = 72.0

type graphvizLayout struct {
	WidthInches  float64
	HeightInches float64
	Nodes        map[string]graphvizNodeLayout
	Edges        map[string]graphvizEdgeLayout
}

type graphvizNodeLayout struct {
	CenterX, CenterY float64
	Width, Height    float64
}

type graphvizEdgeLayout struct {
	Points []graphvizPoint
}

type graphvizPoint struct {
	X, Y float64
}

func layoutFromDOT(dot []byte) (*graphvizLayout, error) {
	cmd := exec.Command("dot", "-Tplain")
	cmd.Stdin = bytes.NewReader(dot)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("dot -Tplain: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return parseGraphvizPlain(out)
}

func renderCompactLayoutDOT(g yedExportGraph) (string, map[string]string) {
	idMap := make(map[string]string, len(g.Nodes))
	compact := make(map[string]string, len(g.Nodes))
	var b strings.Builder
	fmt.Fprintf(&b, "digraph layout {\n")
	fmt.Fprintf(&b, "  graph [rankdir=LR, bgcolor=white];\n")
	fmt.Fprintf(&b, "  node [fontname=Helvetica, fontsize=10, shape=box];\n")
	for i, n := range g.Nodes {
		cid := fmt.Sprintf("g%d", i)
		idMap[cid] = n.ID
		compact[n.ID] = cid
		label := n.Label
		if len(label) > 120 {
			label = label[:117] + "..."
		}
		fmt.Fprintf(&b, "  %s [label=%q];\n", cid, label)
	}
	for _, e := range g.Edges {
		from, ok1 := compact[e.Source]
		to, ok2 := compact[e.Target]
		if !ok1 || !ok2 {
			continue
		}
		fmt.Fprintf(&b, "  %s -> %s;\n", from, to)
	}
	fmt.Fprintf(&b, "}\n")
	return b.String(), idMap
}

func remapLayoutNodeIDs(layout *graphvizLayout, idMap map[string]string) {
	if layout == nil || len(idMap) == 0 {
		return
	}
	newNodes := make(map[string]graphvizNodeLayout, len(layout.Nodes))
	for cid, loc := range layout.Nodes {
		if eid, ok := idMap[cid]; ok {
			newNodes[eid] = loc
			continue
		}
		if eid, ok := idMap[unquotePlainToken(cid)]; ok {
			newNodes[eid] = loc
			continue
		}
		newNodes[cid] = loc
	}
	layout.Nodes = newNodes

	newEdges := make(map[string]graphvizEdgeLayout, len(layout.Edges))
	for key, loc := range layout.Edges {
		parts := strings.Split(key, "\x00")
		if len(parts) != 2 {
			newEdges[key] = loc
			continue
		}
		from := parts[0]
		to := parts[1]
		if eid, ok := idMap[from]; ok {
			from = eid
		} else if eid, ok := idMap[unquotePlainToken(from)]; ok {
			from = eid
		}
		if eid, ok := idMap[to]; ok {
			to = eid
		} else if eid, ok := idMap[unquotePlainToken(to)]; ok {
			to = eid
		}
		newEdges[from+"\x00"+to] = loc
	}
	layout.Edges = newEdges
}

func (l graphvizNodeLayout) yedGeometry(graphHeightInches float64) (x, y, w, h float64) {
	w = l.Width * graphvizInchesToPixels
	h = l.Height * graphvizInchesToPixels
	x = (l.CenterX - l.Width/2) * graphvizInchesToPixels
	y = (graphHeightInches - l.CenterY - l.Height/2) * graphvizInchesToPixels
	return x, y, w, h
}

func (p graphvizPoint) yedPoint(graphHeightInches float64) graphvizPoint {
	return graphvizPoint{
		X: p.X * graphvizInchesToPixels,
		Y: (graphHeightInches - p.Y) * graphvizInchesToPixels,
	}
}

func dotFromGraphJSON(data []byte, opts yedExportOptions) (string, yedExportGraph, error) {
	var probe struct {
		Version *int `json:"version"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return "", yedExportGraph{}, fmt.Errorf("parse json: %w", err)
	}
	if probe.Version != nil {
		var mg MergedGraph
		if err := json.Unmarshal(data, &mg); err != nil {
			return "", yedExportGraph{}, fmt.Errorf("parse merged graph: %w", err)
		}
		return renderMergedDOT(&mg), yedGraphFromMerged(&mg, opts), nil
	}
	var g EndpointGraph
	if err := json.Unmarshal(data, &g); err != nil {
		return "", yedExportGraph{}, fmt.Errorf("parse endpoint graph: %w", err)
	}
	return renderDOTGraph(&g), yedGraphFromEndpoint(&g, opts), nil
}

func renderYedExportFromJSON(data []byte, opts yedExportOptions) ([]byte, error) {
	_, g, err := dotFromGraphJSON(data, opts)
	if err != nil {
		return nil, err
	}
	dot, idMap := renderCompactLayoutDOT(g)
	layout, err := layoutFromDOT([]byte(dot))
	if err != nil {
		return nil, err
	}
	remapLayoutNodeIDs(layout, idMap)
	return renderYedGraphML(g, layout)
}
