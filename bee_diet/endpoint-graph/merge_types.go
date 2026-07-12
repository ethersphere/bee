// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

const mergedGraphVersion = 1

// MergedEndpointRef identifies one source endpoint graph in the union.
type MergedEndpointRef struct {
	ID       string   `json:"id"`
	Dir      string   `json:"dir"`
	Endpoint Endpoint `json:"endpoint"`
}

// MergedNode is a deduplicated graph node with endpoint fan-in.
type MergedNode struct {
	GraphNode
	Endpoints     []string `json:"endpoints"`
	EndpointCount int      `json:"endpoint_count"`
	SortOrder     int      `json:"sort_order"`
}

// MergedEdge is a deduplicated graph edge with endpoint fan-in.
type MergedEdge struct {
	GraphEdge
	Endpoints     []string `json:"endpoints"`
	EndpointCount int      `json:"endpoint_count"`
}

// MergedPackageSummary aggregates package nodes across endpoints.
type MergedPackageSummary struct {
	Package       string   `json:"package"`
	EndpointCount int      `json:"endpoint_count"`
	Endpoints     []string `json:"endpoints"`
}

// MergedGraphLayout hints for rendering the union graph.
type MergedGraphLayout struct {
	Layers    []GraphLayer `json:"layers"`
	NodeTypes []NodeType   `json:"node_types"`
}

// MergedGraphStats summarizes the union graph.
type MergedGraphStats struct {
	EndpointCount int `json:"endpoint_count"`
	NodeCount     int `json:"node_count"`
	EdgeCount     int `json:"edge_count"`
	PackageCount  int `json:"package_count"`
}

// MergedGraph is the union of per-endpoint graphs with endpoint fan-in metadata.
type MergedGraph struct {
	Version          int                    `json:"version"`
	SourceDir        string                 `json:"source_dir"`
	CompressPackages bool                   `json:"compress_packages,omitempty"`
	Stats            MergedGraphStats       `json:"stats"`
	Layout           MergedGraphLayout      `json:"layout"`
	Endpoints        []MergedEndpointRef    `json:"endpoints"`
	Nodes            []MergedNode           `json:"nodes"`
	Edges            []MergedEdge           `json:"edges"`
	Packages         []MergedPackageSummary `json:"packages"`
	EntityProjection *EntityProjection      `json:"entity_projection,omitempty"`
}
