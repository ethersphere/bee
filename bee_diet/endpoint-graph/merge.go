// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type mergeNodeAcc struct {
	node      GraphNode
	endpoints map[string]struct{}
}

type mergeEdgeAcc struct {
	edge      GraphEdge
	endpoints map[string]struct{}
}

func mergedPackageNodeID(pkg string) string {
	return "pkg:" + pkg
}

func graphHasDetailedCallNodes(g EndpointGraph) bool {
	for _, n := range g.Nodes {
		switch n.Type {
		case NodeCapability, NodeEntity, NodeServiceField, NodeImplementation, NodeBackend:
			return true
		}
	}
	return false
}

func shouldCollapsePackagesOnMerge(graphs []loadedEndpointGraph, enabled bool) bool {
	if !enabled {
		return false
	}
	for _, lg := range graphs {
		if lg.graph.DirectCalls {
			return false
		}
	}
	for _, lg := range graphs {
		if graphHasDetailedCallNodes(lg.graph) {
			return false
		}
	}
	return true
}

func collapsePackageNodes(nodesByID map[string]*mergeNodeAcc, edgesByKey map[string]*mergeEdgeAcc) {
	remap := map[string]string{}
	canonical := map[string]*mergeNodeAcc{}

	var packageIDs []string
	for id, acc := range nodesByID {
		if acc.node.Type != NodePackage || acc.node.Package == "" {
			continue
		}
		packageIDs = append(packageIDs, id)
	}

	for _, id := range packageIDs {
		acc := nodesByID[id]
		target := mergedPackageNodeID(acc.node.Package)
		remap[id] = target

		merged, ok := canonical[target]
		if !ok {
			node := acc.node
			node.ID = target
			node.Label = acc.node.Package
			node.Metadata = nil
			merged = &mergeNodeAcc{node: node, endpoints: map[string]struct{}{}}
			canonical[target] = merged
		}
		for ep := range acc.endpoints {
			merged.endpoints[ep] = struct{}{}
		}
		delete(nodesByID, id)
	}

	for id, acc := range canonical {
		nodesByID[id] = acc
	}

	newEdges := map[string]*mergeEdgeAcc{}
	for _, acc := range edgesByKey {
		e := acc.edge
		if to, ok := remap[e.From]; ok {
			e.From = to
		}
		if to, ok := remap[e.To]; ok {
			e.To = to
		}
		key := edgeMergeKey(e)
		existing, ok := newEdges[key]
		if !ok {
			existing = &mergeEdgeAcc{edge: e, endpoints: map[string]struct{}{}}
			newEdges[key] = existing
		}
		for ep := range acc.endpoints {
			existing.endpoints[ep] = struct{}{}
		}
	}
	for k := range edgesByKey {
		delete(edgesByKey, k)
	}
	for k, v := range newEdges {
		edgesByKey[k] = v
	}
}

func mergeGraphs(sourceDir string, graphs []loadedEndpointGraph, collapsePackages bool) (*MergedGraph, error) {
	if len(graphs) == 0 {
		return nil, fmt.Errorf("no graphs to merge")
	}

	type nodeAcc = mergeNodeAcc
	type edgeAcc = mergeEdgeAcc

	nodesByID := map[string]*nodeAcc{}
	edgesByKey := map[string]*edgeAcc{}
	var endpointRefs []MergedEndpointRef

	for _, lg := range graphs {
		epID := endpointKey(lg.graph.Endpoint)
		endpointRefs = append(endpointRefs, MergedEndpointRef{
			ID:       epID,
			Dir:      lg.dir,
			Endpoint: lg.graph.Endpoint,
		})

		for _, n := range lg.graph.Nodes {
			acc, ok := nodesByID[n.ID]
			if !ok {
				acc = &nodeAcc{node: n, endpoints: map[string]struct{}{}}
				nodesByID[n.ID] = acc
			}
			acc.endpoints[epID] = struct{}{}
		}

		for _, e := range lg.graph.Edges {
			key := edgeMergeKey(e)
			acc, ok := edgesByKey[key]
			if !ok {
				acc = &edgeAcc{edge: e, endpoints: map[string]struct{}{}}
				edgesByKey[key] = acc
			}
			acc.endpoints[epID] = struct{}{}
		}
	}

	compressPackages := shouldCollapsePackagesOnMerge(graphs, collapsePackages)
	if compressPackages {
		collapsePackageNodes(nodesByID, edgesByKey)
	}

	sort.Slice(endpointRefs, func(i, j int) bool {
		if endpointRefs[i].Endpoint.Mount != endpointRefs[j].Endpoint.Mount {
			return endpointRefs[i].Endpoint.Mount < endpointRefs[j].Endpoint.Mount
		}
		return endpointRefs[i].ID < endpointRefs[j].ID
	})

	nodes := make([]MergedNode, 0, len(nodesByID))
	for _, acc := range nodesByID {
		eps := sortedKeys(acc.endpoints)
		nodes = append(nodes, MergedNode{
			GraphNode:     acc.node,
			Endpoints:     eps,
			EndpointCount: len(eps),
		})
	}
	sortMergedNodes(nodes)

	edges := make([]MergedEdge, 0, len(edgesByKey))
	for _, acc := range edgesByKey {
		eps := sortedKeys(acc.endpoints)
		edges = append(edges, MergedEdge{
			GraphEdge:     acc.edge,
			Endpoints:     eps,
			EndpointCount: len(eps),
		})
	}
	sortMergedEdges(edges)

	packages := buildMergedPackageSummary(nodes)

	return &MergedGraph{
		Version:          mergedGraphVersion,
		SourceDir:        sourceDir,
		CompressPackages: compressPackages,
		Stats: MergedGraphStats{
			EndpointCount: len(endpointRefs),
			NodeCount:     len(nodes),
			EdgeCount:     len(edges),
			PackageCount:  len(packages),
		},
		Layout:           defaultMergedLayout(),
		Endpoints:        endpointRefs,
		Nodes:            nodes,
		Edges:            edges,
		Packages:         packages,
		EntityProjection: mergeEntityProjections(graphs),
	}, nil
}

type loadedEndpointGraph struct {
	dir   string
	graph EndpointGraph
}

func filterGraphsByMount(graphs []loadedEndpointGraph, mounts []string) []loadedEndpointGraph {
	allow := parseMountNames(mounts)
	if len(allow) == 0 {
		return graphs
	}
	var out []loadedEndpointGraph
	for _, lg := range graphs {
		if _, ok := allow[lg.graph.Endpoint.Mount]; ok {
			out = append(out, lg)
		}
	}
	return out
}

func parseMountNames(mounts []string) map[string]struct{} {
	if len(mounts) == 0 {
		return nil
	}
	allow := map[string]struct{}{}
	for _, mount := range mounts {
		for _, part := range strings.Split(mount, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				allow[part] = struct{}{}
			}
		}
	}
	return allow
}

func loadEndpointGraphs(sourceDir string) ([]loadedEndpointGraph, error) {
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}

	var graphs []loadedEndpointGraph
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		path := filepath.Join(sourceDir, ent.Name(), graphFileBase+".json")
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		var g EndpointGraph
		if err := json.Unmarshal(data, &g); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		graphs = append(graphs, loadedEndpointGraph{dir: ent.Name(), graph: g})
	}

	sort.Slice(graphs, func(i, j int) bool {
		return graphs[i].dir < graphs[j].dir
	})
	return graphs, nil
}

func edgeMergeKey(e GraphEdge) string {
	return e.From + "\x00" + e.To + "\x00" + e.Kind + "\x00" + e.Branch
}

func defaultMergedLayout() MergedGraphLayout {
	return MergedGraphLayout{
		Layers: []GraphLayer{LayerRoute, LayerCall, LayerWiring},
		NodeTypes: []NodeType{
			NodeRoute, NodeMiddleware, NodeHandler, NodeServiceField,
			NodeCapability, NodeEntity, NodePackage, NodeImplementation, NodeBackend, NodeExternalDeps,
		},
	}
}

func mergedLayerRank(layer GraphLayer) int {
	switch layer {
	case LayerRoute:
		return 0
	case LayerCall:
		return 1
	case LayerWiring:
		return 2
	default:
		return 99
	}
}

func mergedTypeRank(t NodeType) int {
	switch t {
	case NodeRoute:
		return 0
	case NodeMiddleware:
		return 1
	case NodeHandler:
		return 2
	case NodeServiceField:
		return 3
	case NodeCapability:
		return 4
	case NodeEntity:
		return 4
	case NodePackage:
		return 5
	case NodeImplementation:
		return 6
	case NodeBackend:
		return 7
	case NodeExternalDeps:
		return 8
	default:
		return 99
	}
}

func sortMergedNodes(nodes []MergedNode) {
	for i := range nodes {
		n := &nodes[i]
		n.SortOrder = mergedLayerRank(n.Layer)*1_000_000 +
			mergedTypeRank(n.Type)*10_000 +
			(10_000 - minInt(n.EndpointCount, 9_999))
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].SortOrder != nodes[j].SortOrder {
			return nodes[i].SortOrder < nodes[j].SortOrder
		}
		return nodes[i].ID < nodes[j].ID
	})
}

func sortMergedEdges(edges []MergedEdge) {
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].To != edges[j].To {
			return edges[i].To < edges[j].To
		}
		if edges[i].Kind != edges[j].Kind {
			return edges[i].Kind < edges[j].Kind
		}
		if edges[i].Branch != edges[j].Branch {
			return edges[i].Branch < edges[j].Branch
		}
		return edges[i].EndpointCount > edges[j].EndpointCount
	})
}

func buildMergedPackageSummary(nodes []MergedNode) []MergedPackageSummary {
	byPkg := map[string]map[string]struct{}{}
	for _, n := range nodes {
		if n.Type != NodePackage && n.Type != NodeHandler && n.Type != NodeImplementation {
			continue
		}
		pkg := n.Package
		if pkg == "" {
			pkg = n.Label
		}
		if pkg == "" || !strings.HasPrefix(pkg, "pkg/") {
			continue
		}
		if _, ok := byPkg[pkg]; !ok {
			byPkg[pkg] = map[string]struct{}{}
		}
		for _, ep := range n.Endpoints {
			byPkg[pkg][ep] = struct{}{}
		}
	}

	out := make([]MergedPackageSummary, 0, len(byPkg))
	for pkg, eps := range byPkg {
		list := sortedKeys(eps)
		out = append(out, MergedPackageSummary{
			Package:       pkg,
			Endpoints:     list,
			EndpointCount: len(list),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].EndpointCount != out[j].EndpointCount {
			return out[i].EndpointCount > out[j].EndpointCount
		}
		return out[i].Package < out[j].Package
	})
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func writeMergedGraph(path string, mg *MergedGraph) error {
	data, err := json.MarshalIndent(mg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
