// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

type yedExportGraph struct {
	Title string
	Nodes []yedExportNode
	Edges []yedExportEdge
}

type yedExportNode struct {
	ID          string
	Label       string
	Description string
	NodeType    string
	Layer       string
	Shape       string
	FillColor   string
}

type yedExportEdge struct {
	ID          string
	Source      string
	Target      string
	Label       string
	Description string
}

type yedExportOptions struct {
	EssentialColor bool
}

func yedGraphFromEndpoint(g *EndpointGraph, opts yedExportOptions) yedExportGraph {
	title := strings.TrimSpace(g.Endpoint.Method + " " + g.Endpoint.Path)
	if title == "" {
		title = g.Endpoint.Handler
	}
	out := yedExportGraph{Title: title}
	for _, n := range g.Nodes {
		out.Nodes = append(out.Nodes, yedExportNode{
			ID:          graphMLNodeID(n.ID),
			Label:       n.Label,
			Description: yedEndpointNodeDescription(n),
			NodeType:    string(n.Type),
			Layer:       string(n.Layer),
			Shape:       yedShape(n.Type),
			FillColor:   yedFillColorForNode(n, opts.EssentialColor),
		})
	}
	for i, e := range g.Edges {
		out.Edges = append(out.Edges, yedExportEdge{
			ID:          fmt.Sprintf("e%d", i),
			Source:      graphMLNodeID(e.From),
			Target:      graphMLNodeID(e.To),
			Label:       yedEdgeLabel(e.Kind, e.Branch),
			Description: e.Kind,
		})
	}
	return out
}

func yedGraphFromMerged(mg *MergedGraph, opts yedExportOptions) yedExportGraph {
	title := "merged endpoint graph"
	if mg.SourceDir != "" {
		title = "merged: " + mg.SourceDir
	}
	out := yedExportGraph{Title: title}
	for _, n := range mg.Nodes {
		label := n.Label
		if n.EndpointCount > 1 {
			label += fmt.Sprintf("\n(%d endpoints)", n.EndpointCount)
		}
		out.Nodes = append(out.Nodes, yedExportNode{
			ID:          graphMLNodeID(n.ID),
			Label:       label,
			Description: mergedNodeTooltip(n),
			NodeType:    string(n.Type),
			Layer:       string(n.Layer),
			Shape:       yedShape(n.Type),
			FillColor:   yedFillColorForNode(n.GraphNode, opts.EssentialColor),
		})
	}
	for i, e := range mg.Edges {
		label := yedEdgeLabel(e.Kind, e.Branch)
		if e.EndpointCount > 1 {
			label += fmt.Sprintf(" (%d)", e.EndpointCount)
		}
		desc := strings.Join(e.Endpoints, "\n")
		out.Edges = append(out.Edges, yedExportEdge{
			ID:          fmt.Sprintf("e%d", i),
			Source:      graphMLNodeID(e.From),
			Target:      graphMLNodeID(e.To),
			Label:       label,
			Description: desc,
		})
	}
	return out
}

func loadYedExportGraph(path string) (yedExportGraph, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return yedExportGraph{}, err
	}
	return parseYedExportGraph(data)
}

func parseYedExportGraph(data []byte) (yedExportGraph, error) {
	_, g, err := dotFromGraphJSON(data, yedExportOptions{EssentialColor: false})
	return g, err
}

func renderYedGraphML(g yedExportGraph, layout *graphvizLayout) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="no"?>` + "\n")
	b.WriteString(`<graphml xmlns="http://graphml.graphdrawing.org/xmlns"` + "\n")
	b.WriteString(`         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"` + "\n")
	b.WriteString(`         xmlns:y="http://www.yworks.com/xml/graphml"` + "\n")
	b.WriteString(`         xsi:schemaLocation="http://graphml.graphdrawing.org/xmlns http://www.yworks.com/xml/schema/graphml/1.1/ygraphml.xsd">` + "\n")
	b.WriteString(`  <key for="node" id="d0" yfiles.type="nodegraphics"/>` + "\n")
	b.WriteString(`  <key for="edge" id="d1" yfiles.type="edgegraphics"/>` + "\n")
	b.WriteString(`  <key for="graph" id="d2" yfiles.type="resources"/>` + "\n")
	b.WriteString(`  <key for="graphml" id="d3" yfiles.type="resources"/>` + "\n")
	b.WriteString(`  <key attr.name="description" attr.type="string" for="node" id="d4"/>` + "\n")
	b.WriteString(`  <key attr.name="description" attr.type="string" for="edge" id="d5"/>` + "\n")
	b.WriteString(`  <key attr.name="type" attr.type="string" for="node" id="d6"/>` + "\n")
	b.WriteString(`  <key attr.name="layer" attr.type="string" for="node" id="d7"/>` + "\n")
	fmt.Fprintf(&b, "  <graph id=\"%s\" edgedefault=\"directed\">\n", xmlEscapeAttr(graphMLGraphID(g.Title)))

	for i, n := range g.Nodes {
		x, y, w, h := yedFallbackGeometry(i, n.Label)
		if layout != nil {
			if loc, ok := layout.Nodes[n.ID]; ok {
				x, y, w, h = loc.yedGeometry(layout.HeightInches)
			}
		}
		fmt.Fprintf(&b, "    <node id=\"%s\">\n", xmlEscapeAttr(n.ID))
		if n.Description != "" {
			fmt.Fprintf(&b, "      <data key=\"d4\">%s</data>\n", xmlEscapeText(n.Description))
		}
		if n.NodeType != "" {
			fmt.Fprintf(&b, "      <data key=\"d6\">%s</data>\n", xmlEscapeText(n.NodeType))
		}
		if n.Layer != "" {
			fmt.Fprintf(&b, "      <data key=\"d7\">%s</data>\n", xmlEscapeText(n.Layer))
		}
		b.WriteString("      <data key=\"d0\">\n")
		b.WriteString("        <y:ShapeNode>\n")
		fmt.Fprintf(&b, "          <y:Geometry height=\"%.1f\" width=\"%.1f\" x=\"%.1f\" y=\"%.1f\"/>\n", h, w, x, y)
		fmt.Fprintf(&b, "          <y:Fill color=\"%s\" transparent=\"false\"/>\n", xmlEscapeAttr(n.FillColor))
		b.WriteString("          <y:BorderStyle color=\"#000000\" type=\"line\" width=\"1.0\"/>\n")
		fmt.Fprintf(&b, "          <y:NodeLabel alignment=\"center\" autoSizePolicy=\"content\" fontSize=\"12\" textColor=\"#000000\">%s</y:NodeLabel>\n",
			xmlEscapeText(n.Label))
		fmt.Fprintf(&b, "          <y:Shape type=\"%s\"/>\n", xmlEscapeAttr(n.Shape))
		b.WriteString("        </y:ShapeNode>\n")
		b.WriteString("      </data>\n")
		b.WriteString("    </node>\n")
	}

	for _, e := range g.Edges {
		fmt.Fprintf(&b, "    <edge id=\"%s\" source=\"%s\" target=\"%s\">\n",
			xmlEscapeAttr(e.ID), xmlEscapeAttr(e.Source), xmlEscapeAttr(e.Target))
		if e.Description != "" {
			fmt.Fprintf(&b, "      <data key=\"d5\">%s</data>\n", xmlEscapeText(e.Description))
		}
		b.WriteString("      <data key=\"d1\">\n")
		b.WriteString("        <y:PolyLineEdge>\n")
		if layout != nil {
			if edgeLayout, ok := layout.Edges[e.Source+"\x00"+e.Target]; ok && len(edgeLayout.Points) >= 2 {
				writeYedEdgePath(&b, edgeLayout, layout.HeightInches)
			} else {
				b.WriteString("          <y:Path sx=\"0.0\" sy=\"0.0\" tx=\"0.0\" ty=\"0.0\"/>\n")
			}
		} else {
			b.WriteString("          <y:Path sx=\"0.0\" sy=\"0.0\" tx=\"0.0\" ty=\"0.0\"/>\n")
		}
		b.WriteString("          <y:LineStyle color=\"#000000\" type=\"line\" width=\"1.0\"/>\n")
		if e.Label != "" {
			fmt.Fprintf(&b, "          <y:EdgeLabel alignment=\"center\" fontSize=\"10\" textColor=\"#404040\">%s</y:EdgeLabel>\n",
				xmlEscapeText(e.Label))
		}
		b.WriteString("          <y:BendStyle smoothed=\"false\"/>\n")
		b.WriteString("        </y:PolyLineEdge>\n")
		b.WriteString("      </data>\n")
		b.WriteString("    </edge>\n")
	}

	b.WriteString("  </graph>\n")
	b.WriteString("</graphml>\n")

	if err := validateGraphML(b.Bytes()); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func validateGraphML(data []byte) error {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		_, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("invalid graphml: %w", err)
		}
	}
}

func graphMLNodeID(id string) string {
	return dotID(id)
}

func graphMLGraphID(title string) string {
	id := dotID(title)
	if id == "" || id == "n" {
		return "G"
	}
	return id
}

func yedShape(t NodeType) string {
	switch t {
	case NodeEntry, NodeRoute:
		return "ellipse"
	case NodeFunction, NodeMiddleware, NodeHandler, NodePackage, NodeExternalDeps:
		return "rectangle"
	case NodeServiceField:
		return "diamond"
	case NodeCapability, NodeEntity:
		return "hexagon"
	case NodeImplementation:
		return "parallelogram"
	case NodeBackend:
		return "roundrectangle"
	default:
		return "rectangle"
	}
}

func yedFillColor(layer GraphLayer) string {
	switch layer {
	case LayerRoute:
		return "#FFFFCC"
	case LayerCall:
		return "#F5DEB3"
	case LayerWiring:
		return "#ADD8E6"
	default:
		return "#FFFFFF"
	}
}

func yedFallbackGeometry(index int, label string) (x, y, w, h float64) {
	w = yedNodeWidth(label)
	h = yedNodeHeight(label)
	x = float64(index%12) * (w + 48)
	y = float64(index/12) * (h + 48)
	return x, y, w, h
}

func writeYedEdgePath(b *bytes.Buffer, edge graphvizEdgeLayout, graphHeightInches float64) {
	b.WriteString("          <y:Path sx=\"0.0\" sy=\"0.0\" tx=\"0.0\" ty=\"0.0\">\n")
	for _, pt := range edge.Points {
		p := pt.yedPoint(graphHeightInches)
		fmt.Fprintf(b, "            <y:Point x=\"%.1f\" y=\"%.1f\"/>\n", p.X, p.Y)
	}
	b.WriteString("          </y:Path>\n")
}

func yedNodeWidth(label string) float64 {
	lines := strings.Split(label, "\n")
	maxLen := 0
	for _, line := range lines {
		if len(line) > maxLen {
			maxLen = len(line)
		}
	}
	w := float64(maxLen)*7.5 + 24
	if w < 80 {
		return 80
	}
	if w > 360 {
		return 360
	}
	return w
}

func yedNodeHeight(label string) float64 {
	lines := strings.Count(label, "\n") + 1
	h := float64(lines)*18 + 16
	if h < 40 {
		return 40
	}
	return h
}

func yedEdgeLabel(kind, branch string) string {
	if branch == "" {
		return kind
	}
	return kind + " / " + branch
}

func yedEndpointNodeDescription(n GraphNode) string {
	var parts []string
	if n.ID != "" {
		parts = append(parts, "id: "+n.ID)
	}
	if n.Package != "" {
		parts = append(parts, "package: "+n.Package)
	}
	if n.Interface != "" {
		parts = append(parts, "interface: "+n.Interface)
	}
	if n.Method != "" {
		parts = append(parts, "method: "+n.Method)
	}
	if n.Field != "" {
		parts = append(parts, "field: "+n.Field)
	}
	if n.Op != "" {
		parts = append(parts, "op: "+n.Op)
	}
	if len(n.Metadata) > 0 {
		keys := make([]string, 0, len(n.Metadata))
		for k := range n.Metadata {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			parts = append(parts, k+": "+n.Metadata[k])
		}
	}
	return strings.Join(parts, "\n")
}

func xmlEscapeText(s string) string {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(s)); err != nil {
		return strconv.Quote(s)
	}
	return b.String()
}

func xmlEscapeAttr(s string) string {
	return xmlEscapeText(s)
}

func writeYedExportFile(path string, data []byte, opts yedExportOptions) error {
	out, err := renderYedExportFromJSON(data, opts)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}
