// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"sort"
	"strings"
)

func buildSharedIndex(graphs []*EndpointGraph) SharedIndex {
	type acc struct {
		label     string
		pkg       string
		iface     string
		endpoints map[string]struct{}
		mounts    map[string]struct{}
	}
	byCap := map[string]*acc{}

	for _, g := range graphs {
		epKey := endpointKey(g.Endpoint)
		for _, n := range g.Nodes {
			if n.Type != NodeCapability && n.Type != NodeEntity && n.Type != NodePackage {
				continue
			}
			// Package nodes from static call walks are not subsystem fan-in targets.
			if n.Type == NodePackage {
				hasInvoke := false
				for _, e := range g.Edges {
					if e.To == n.ID && e.Kind == "invokes" {
						hasInvoke = true
						break
					}
				}
				if !hasInvoke {
					continue
				}
			}
			a, ok := byCap[n.ID]
			if !ok {
				a = &acc{
					label: n.Label, pkg: n.Package, iface: n.Interface,
					endpoints: map[string]struct{}{}, mounts: map[string]struct{}{},
				}
				byCap[n.ID] = a
			}
			a.endpoints[epKey] = struct{}{}
			a.mounts[g.Endpoint.Mount] = struct{}{}
		}
	}

	var caps []SharedCapability
	for id, a := range byCap {
		caps = append(caps, SharedCapability{
			ID: id, Label: a.label, Endpoints: sortedKeys(a.endpoints),
			Mounts: sortedKeys(a.mounts), Count: len(a.endpoints),
			Package: a.pkg, Interface: a.iface,
		})
	}
	sort.Slice(caps, func(i, j int) bool {
		if caps[i].Count != caps[j].Count {
			return caps[i].Count > caps[j].Count
		}
		return caps[i].ID < caps[j].ID
	})
	return SharedIndex{Capabilities: caps}
}

func writeIndexMD(path string, graphs []*EndpointGraph) error {
	var b strings.Builder
	b.WriteString("# Endpoint capability graphs (endpoint-graph)\n\n")
	b.WriteString("| Mount | Method | Path | Handler | Middlewares | Capabilities | SVG |\n")
	b.WriteString("|-------|--------|------|---------|-------------|--------------|-----|\n")
	for _, g := range graphs {
		mw := strings.Join(g.Endpoint.Middlewares, ", ")
		if mw == "" {
			mw = "—"
		}
		caps := strings.Join(g.CapabilityPackages, ", ")
		if len(caps) > 80 {
			caps = caps[:77] + "..."
		}
		dir := EndpointDirName(g.Endpoint)
		svg := dir + "/" + graphFileBase + ".svg"
		b.WriteString("| " + g.Endpoint.Mount + " | " + g.Endpoint.Method + " | `" + g.Endpoint.Path +
			"` | `" + g.Endpoint.Handler + "` | " + mw + " | " + caps + " | [" + svg + "](" + svg + ") |\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
