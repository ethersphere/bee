// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

const beeModule = "github.com/ethersphere/bee/v2"

// Endpoint describes one HTTP route from router.go.
type Endpoint struct {
	Mount       string   `json:"mount"`
	Method      string   `json:"method"`
	Path        string   `json:"path"`
	PathV1      string   `json:"path_v1,omitempty"`
	Handler     string   `json:"handler"`
	Middlewares []string `json:"middlewares,omitempty"`
	Conditional bool     `json:"conditional,omitempty"`
	External    bool     `json:"external,omitempty"`
}

// NodeType classifies graph nodes for manual dependency analysis.
type NodeType string

const (
	NodeEntry          NodeType = "entry"
	NodeRoute          NodeType = "route"
	NodeFunction       NodeType = "function"
	NodeMiddleware     NodeType = "middleware"
	NodeHandler        NodeType = "handler"
	NodeServiceField   NodeType = "service_field"
	NodeCapability     NodeType = "capability"
	NodeEntity         NodeType = "entity"
	NodePackage        NodeType = "package"
	NodeImplementation NodeType = "implementation"
	NodeBackend        NodeType = "backend"
	NodeExternalDeps   NodeType = "external_deps"
)

// GraphLayer separates route/call/wiring concerns in one graph.
type GraphLayer string

const (
	LayerRoute  GraphLayer = "route"
	LayerCall   GraphLayer = "call"
	LayerWiring GraphLayer = "wiring"
)

// GraphNode is a typed vertex in the endpoint dependency graph.
type GraphNode struct {
	ID        string            `json:"id"`
	Type      NodeType          `json:"type"`
	Label     string            `json:"label"`
	Layer     GraphLayer        `json:"layer"`
	Package   string            `json:"package,omitempty"`
	Interface string            `json:"interface,omitempty"`
	Method    string            `json:"method,omitempty"`
	Field     string            `json:"field,omitempty"`
	Op        string            `json:"op,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// GraphEdge is a typed directed edge.
type GraphEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Kind   string `json:"kind"`
	Branch string `json:"branch,omitempty"`
}

// EntityRef is a runtime component in the entity dependency projection.
type EntityRef struct {
	ID        string            `json:"id"`
	Label     string            `json:"label"`
	Package   string            `json:"package,omitempty"`
	Interface string            `json:"interface,omitempty"`
	Field     string            `json:"field,omitempty"`
	Impl      string            `json:"impl,omitempty"`
	Methods   []string          `json:"methods,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// EntityDep is a directed dependency between runtime entities.
type EntityDep struct {
	From     string            `json:"from"`
	To       string            `json:"to"`
	Kind     string            `json:"kind"`
	Methods  []string          `json:"methods,omitempty"`
	Branch   string            `json:"branch,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// EntityUnresolved records an invoke site that could not be resolved to a concrete entity.
type EntityUnresolved struct {
	Site   string `json:"site"`
	Iface  string `json:"iface,omitempty"`
	Method string `json:"method,omitempty"`
	Reason string `json:"reason"`
	Count  int    `json:"count,omitempty"`
}

// EntityProjection is an additive runtime-entity view over the raw call graph.
type EntityProjection struct {
	Nodes      []EntityRef        `json:"nodes"`
	Edges      []EntityDep        `json:"edges"`
	Unresolved []EntityUnresolved `json:"unresolved,omitempty"`
}

// ProgramEntry describes the SSA root for a whole-program graph (-entry).
type ProgramEntry struct {
	Package  string `json:"package"`
	Function string `json:"function"`
	Source   string `json:"source,omitempty"`
}

// ProgramGraph is the analysis result for a program entry point (e.g. main).
type ProgramGraph struct {
	Entry              ProgramEntry      `json:"entry"`
	Nodes              []GraphNode       `json:"nodes"`
	Edges              []GraphEdge       `json:"edges"`
	CapabilityPackages []string          `json:"capability_packages"`
	StaticPackages     []string          `json:"static_packages,omitempty"`
	ProtocolPackages   []string          `json:"protocol_packages,omitempty"`
	DirectCalls        bool              `json:"direct_calls,omitempty"`
	CompressPackages   bool              `json:"compress_packages,omitempty"`
	EntityProjection   *EntityProjection `json:"entity_projection,omitempty"`
}

// EndpointGraph is the v2 analysis result for one HTTP endpoint.
type EndpointGraph struct {
	Endpoint           Endpoint          `json:"endpoint"`
	Nodes              []GraphNode       `json:"nodes"`
	Edges              []GraphEdge       `json:"edges"`
	CapabilityPackages []string          `json:"capability_packages"`
	DirectCalls        bool              `json:"direct_calls,omitempty"`
	CompressPackages   bool              `json:"compress_packages,omitempty"`
	EntityProjection   *EntityProjection `json:"entity_projection,omitempty"`
}

// ManifestEntry is written to index.json for one endpoint.
type ManifestEntry struct {
	Endpoint           Endpoint `json:"endpoint"`
	Handler            string   `json:"handler"`
	CapabilityPackages []string `json:"capability_packages"`
	Graph              string   `json:"graph_json"`
	SVG                string   `json:"svg,omitempty"`
}

// SharedCapability records cross-endpoint fan-in for manual classification.
type SharedCapability struct {
	ID        string   `json:"id"`
	Label     string   `json:"label"`
	Endpoints []string `json:"endpoints"`
	Mounts    []string `json:"mounts"`
	Count     int      `json:"count"`
	Package   string   `json:"package,omitempty"`
	Interface string   `json:"interface,omitempty"`
}

// SharedIndex aggregates capability fan-in across all endpoints.
type SharedIndex struct {
	Capabilities []SharedCapability `json:"capabilities"`
}
