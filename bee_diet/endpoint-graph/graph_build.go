// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/ssa"
)

type graphBuilder struct {
	prog             *ssa.Program
	idx              *invokeIndex
	wiring           *wiringIndex
	fields           map[string]serviceField
	ep               Endpoint
	entry            ProgramEntry
	programMode      bool
	nodes            map[string]GraphNode
	edges            []GraphEdge
	edgeSet          map[string]struct{}
	pkgSeen          map[string]struct{}
	capPkgSeen       map[string]struct{}
	fnVisited        map[string]struct{}
	entityEvents     []entityEvent
	maxDepth         int
	directCalls      bool
	compressPackages bool
	callScope        string
}

func analyzeEndpointGraph(prog *ssa.Program, ep Endpoint, fields map[string]serviceField, maxDepth int, directCalls, compressPackages bool) (*EndpointGraph, error) {
	g := &graphBuilder{
		prog:             prog,
		idx:              newInvokeIndex(prog, fields),
		wiring:           newWiringIndex(),
		fields:           fields,
		ep:               ep,
		nodes:            make(map[string]GraphNode),
		edgeSet:          make(map[string]struct{}),
		pkgSeen:          make(map[string]struct{}),
		capPkgSeen:       make(map[string]struct{}),
		fnVisited:        make(map[string]struct{}),
		maxDepth:         maxDepth,
		directCalls:      directCalls,
		compressPackages: compressPackages && !directCalls,
		callScope:        handlerCallRef(ep.Handler),
	}

	g.addRouteLayer()

	if isExternalHandler(ep.Handler) || ep.External {
		return g.result(), nil
	}

	roots := findHandlerFuncs(prog, ep.Handler)
	if len(roots) == 0 {
		return g.result(), nil
	}

	for _, fn := range roots {
		g.fnVisited = make(map[string]struct{})
		g.callScope = handlerCallRef(g.ep.Handler)
		parent := g.addHandlerNode()
		g.walkCall(fn, parent, 0, "success")
	}

	return g.result(), nil
}

func analyzeProgramGraph(prog *ssa.Program, spec entrySpec, fields map[string]serviceField, maxDepth int, directCalls, compressPackages bool) (*ProgramGraph, error) {
	roots := findEntryFunctions(prog, spec)
	if len(roots) == 0 {
		return nil, fmt.Errorf("entry function not found: %s.%s", spec.PkgRel, spec.FuncName)
	}
	sortEntryFunctions(roots)
	root := roots[0]
	entryRef := fnCallRef(root)
	g := &graphBuilder{
		prog:             prog,
		idx:              newInvokeIndex(prog, fields),
		wiring:           newWiringIndex(),
		fields:           fields,
		entry:            ProgramEntry{Package: spec.PkgRel, Function: spec.FuncName, Source: spec.Source},
		programMode:      true,
		nodes:            make(map[string]GraphNode),
		edgeSet:          make(map[string]struct{}),
		pkgSeen:          make(map[string]struct{}),
		capPkgSeen:       make(map[string]struct{}),
		fnVisited:        make(map[string]struct{}),
		maxDepth:         maxDepth,
		directCalls:      directCalls,
		compressPackages: compressPackages && !directCalls,
		callScope:        entryRef,
	}

	entryID := g.addEntryNode(entryRef)
	g.walkCall(root, entryID, 0, "success")
	return g.programResult(), nil
}

func (g *graphBuilder) addEntryNode(entryRef string) string {
	eid := "entry:" + g.entry.Function
	g.addNode(GraphNode{
		ID: eid, Type: NodeEntry, Layer: LayerRoute,
		Label: entryRef, Package: g.entry.Package,
		Metadata: map[string]string{
			"function": g.entry.Function,
			"source":   g.entry.Source,
		},
	})
	return eid
}

func (g *graphBuilder) pushScope(scope string) func() {
	prev := g.callScope
	g.callScope = scope
	return func() { g.callScope = prev }
}

func (g *graphBuilder) addRouteLayer() {
	rid := "route:" + endpointKey(g.ep)
	g.addNode(GraphNode{
		ID: rid, Type: NodeRoute, Layer: LayerRoute,
		Label: g.ep.Method + " " + g.ep.Path,
		Metadata: map[string]string{
			"mount": g.ep.Mount,
		},
	})
	if g.ep.Conditional {
		n := g.nodes[rid]
		if n.Metadata == nil {
			n.Metadata = map[string]string{}
		}
		n.Metadata["conditional"] = "true"
		g.nodes[rid] = n
	}

	prev := rid
	_ = prev
	hid := "handler:" + g.ep.Handler
	g.addNode(GraphNode{
		ID: hid, Type: NodeHandler, Layer: LayerRoute,
		Label: handlerCallRef(g.ep.Handler), Package: apiHandlerPkg,
	})
	g.linkEdge(rid, hid, "wraps", "")
}

func (g *graphBuilder) fnVisitKey(fn *ssa.Function) string {
	return fnCallRef(fn) + "\x00" + g.callScope
}

func (g *graphBuilder) addHandlerNode() string {
	g.pkgSeen[apiHandlerPkg] = struct{}{}
	return "handler:" + g.ep.Handler
}

func (g *graphBuilder) walkCall(fn *ssa.Function, parentID string, depth int, branch string) {
	if fn == nil {
		return
	}
	if g.maxDepth > 0 && depth > g.maxDepth {
		return
	}
	visitKey := g.fnVisitKey(fn)
	if visitKey != "" {
		if _, seen := g.fnVisited[visitKey]; seen {
			return
		}
		g.fnVisited[visitKey] = struct{}{}
	}

	if pkg, ok := g.trackPackage(fn); ok {
		g.pkgSeen[pkg] = struct{}{}
	}

	for _, block := range fn.Blocks {
		for _, ins := range block.Instrs {
			call, ok := ins.(ssa.CallInstruction)
			if !ok {
				continue
			}
			g.processCall(call, parentID, fn, depth, branch)
		}
	}
}

func (g *graphBuilder) trackPackage(fn *ssa.Function) (string, bool) {
	if g.programMode {
		rel, ok := beeProjectPath(fn)
		if !ok || !strings.HasPrefix(rel, "pkg/") {
			return "", false
		}
		return rel, true
	}
	return beePkgPath(fn)
}

func (g *graphBuilder) processCall(call ssa.CallInstruction, parentID string, caller *ssa.Function, depth int, branch string) {
	if call.Common().IsInvoke() {
		g.processInvoke(call, parentID, caller, depth, branch)
		return
	}
	callee := call.Common().StaticCallee()
	if callee == nil {
		return
	}
	pkg, isBee := g.calleePackage(callee)
	if !isBee {
		return
	}
	if g.directCalls {
		if strings.HasPrefix(pkg, "pkg/") {
			g.pkgSeen[pkg] = struct{}{}
		}
		nextParent := parentID
		if g.programMode {
			nextParent = g.linkFunctionCall(parentID, callee, pkg, branch)
			if entry, ok := g.wiring.lookupConstructor(constructorRef(callee)); ok {
				g.attachWiringFromEntry(nextParent, entry, fnCallRef(callee))
			}
			if root := protocolChunkRoot(pkg); root != "" {
				g.linkStaticPackage(nextParent, root, branch)
			}
		}
		if _, seen := g.fnVisited[g.fnVisitKey(callee)]; seen {
			return
		}
		restore := g.pushScope(fnCallRef(callee))
		g.walkCall(callee, nextParent, depth+1, branch)
		restore()
		return
	}
	if parentPkg, parentCaller, ok := g.nodePackageContext(parentID); ok &&
		parentPkg == pkg && parentCaller == g.callScope {
		g.walkCall(callee, parentID, depth+1, branch)
		return
	}
	pkgID := g.linkPackage(parentID, pkg, "calls", branch)
	g.walkCall(callee, pkgID, depth+1, branch)
}

func (g *graphBuilder) linkFunctionCall(parentID string, fn *ssa.Function, pkg, branch string) string {
	target := fnCallRef(fn)
	fid := scopedNodeID(g.callScope, "fn", target)
	if _, exists := g.nodes[fid]; !exists {
		g.addNode(GraphNode{
			ID: fid, Type: NodeFunction, Layer: LayerCall,
			Label: scopedLabel(target, g.callScope), Package: pkg,
			Metadata: callerMetadata(g.callScope),
		})
	}
	g.linkEdge(parentID, fid, "calls", branch)
	return fid
}

func (g *graphBuilder) linkStaticPackage(parentID, pkg, branch string) {
	id := scopedNodeID(g.callScope, "staticpkg", pkg)
	if _, exists := g.nodes[id]; !exists {
		g.addNode(GraphNode{
			ID: id, Type: NodePackage, Layer: LayerCall,
			Label: scopedLabel(pkg, g.callScope), Package: pkg,
			Metadata: mergeMetadata(callerMetadata(g.callScope), map[string]string{"reach": "static"}),
		})
	}
	g.linkEdge(parentID, id, "static_reach", branch)
}

func (g *graphBuilder) calleePackage(fn *ssa.Function) (string, bool) {
	if g.programMode {
		return beeProjectPath(fn)
	}
	return beePkgPath(fn)
}

func (g *graphBuilder) processInvoke(call ssa.CallInstruction, parentID string, caller *ssa.Function, depth int, branch string) {
	method := call.Common().Method
	if method == nil {
		return
	}
	ifaceType := call.Common().Value.Type()
	if ifaceType == nil {
		return
	}
	ifaceKey := types.TypeString(ifaceType, nil)
	if !isBeeProjectInterface(ifaceKey) {
		return
	}
	fieldName := g.invokeFieldName(call, ifaceKey, caller)
	if !g.shouldRecordInvoke(ifaceKey, fieldName) {
		return
	}

	res := g.idx.implementations(call, ifaceType)
	g.recordEntityInvoke(call, caller, fieldName, ifaceKey, branch, res)

	shortIface := shortBeeType(ifaceKey)
	capCaller := g.callScope
	pkg := packageFromInterface(ifaceKey)
	if pkg == "" {
		return
	}
	g.pkgSeen[pkg] = struct{}{}

	if g.compressPackages {
		pkgID := g.linkPackage(parentID, pkg, "invokes", branch)
		g.walkInvokeImplementations(res, pkgID, depth, branch)
		return
	}

	var capTarget string
	var capID string
	var nodeType NodeType
	var methodName string
	capTarget = capabilityTarget(shortIface, method.Name())
	capID = scopedNodeID(capCaller, "cap", capTarget)
	nodeType = NodeCapability
	methodName = method.Name()

	meta := callerMetadata(capCaller)
	capParent := parentID
	if fieldName != "" {
		meta = mergeMetadata(meta, map[string]string{"field": fieldName})
		if sf, ok := g.fields[fieldName]; ok {
			fieldTarget := serviceFieldTarget(sf)
			fid := scopedNodeID(capCaller, "field", fieldTarget)
			if _, exists := g.nodes[fid]; !exists {
				g.addNode(GraphNode{
					ID: fid, Type: NodeServiceField, Layer: LayerCall,
					Label: scopedLabel(fieldTarget, capCaller),
					Field: sf.Field, Interface: shortBeeType(sf.Interface), Package: sf.Package,
					Metadata: mergeMetadata(callerMetadata(capCaller), map[string]string{"field": fieldName}),
				})
			}
			g.linkEdge(parentID, fid, "uses", branch)
			capParent = fid
		}
	}

	if _, exists := g.nodes[capID]; !exists {
		g.addNode(GraphNode{
			ID: capID, Type: nodeType, Layer: LayerCall,
			Label:     scopedLabel(capTarget, capCaller),
			Interface: shortIface, Method: methodName, Field: fieldName,
			Package: packageFromInterface(ifaceKey), Op: inferOp(method.Name()), Metadata: meta,
		})
	}
	g.linkEdge(capParent, capID, "invokes", branch)

	if pkg := packageFromInterface(ifaceKey); pkg != "" {
		g.pkgSeen[pkg] = struct{}{}
		g.capPkgSeen[pkg] = struct{}{}
	}

	capScope := capTarget
	g.attachWiring(capID, ifaceKey, capScope)

	for _, impl := range res.impls {
		implPkg, ok := beePkgPath(impl)
		if !ok {
			continue
		}
		nextParent := capID
		wiringID := scopedNodeID(capScope, "wiring", implPkg)
		if g.hasEdgeBetween(capID, wiringID) {
			nextParent = wiringID
		} else if !g.directCalls {
			pkgID := scopedNodeID(capScope, "pkg", implPkg)
			if !g.hasEdgeBetween(capID, pkgID) {
				pkgID = g.linkPackageUnder(capID, implPkg, capScope, "calls", branch)
			}
			nextParent = pkgID
		} else {
			if strings.HasPrefix(implPkg, "pkg/") {
				g.pkgSeen[implPkg] = struct{}{}
			}
		}
		restore := g.pushScope(capScope)
		if _, seen := g.fnVisited[g.fnVisitKey(impl)]; seen {
			restore()
			continue
		}
		g.walkCall(impl, nextParent, depth+1, branch)
		restore()
	}
}

func (g *graphBuilder) walkInvokeImplementations(res invokeResolution, parentID string, depth int, branch string) {
	for _, impl := range res.impls {
		if _, ok := beePkgPath(impl); !ok {
			continue
		}
		restore := g.pushScope(fnCallRef(impl))
		if _, seen := g.fnVisited[g.fnVisitKey(impl)]; seen {
			restore()
			continue
		}
		g.walkCall(impl, parentID, depth+1, branch)
		restore()
	}
}

func (g *graphBuilder) invokeFieldName(call ssa.CallInstruction, ifaceKey string, caller *ssa.Function) string {
	name := serviceFieldFromValue(call.Common().Value, g.idx)
	if name != "" && isCapabilityField(name) {
		return name
	}
	if callerPkg, ok := beePkgPath(caller); ok && callerPkg == "pkg/api" {
		if f := fieldForInterface(ifaceKey, g.fields); f != "" {
			return f
		}
	}
	return ""
}

func (g *graphBuilder) nodePackageContext(id string) (pkg, caller string, ok bool) {
	n, exists := g.nodes[id]
	if !exists || n.Package == "" {
		return "", "", false
	}
	caller = g.callScope
	if n.Metadata != nil && n.Metadata["caller"] != "" {
		caller = n.Metadata["caller"]
	}
	return n.Package, caller, true
}

func (g *graphBuilder) linkPackage(parentID, pkg, kind, branch string) string {
	return g.linkPackageUnder(parentID, pkg, g.callScope, kind, branch)
}

func (g *graphBuilder) linkPackageUnder(parentID, pkg, caller, kind, branch string) string {
	id := scopedNodeID(caller, "pkg", pkg)
	if id == parentID {
		return parentID
	}
	g.ensureScopedPackageNode(id, pkg, caller, NodePackage, LayerCall, nil)
	g.linkEdge(parentID, id, kind, branch)
	g.pkgSeen[pkg] = struct{}{}
	return id
}

func (g *graphBuilder) ensureScopedPackageNode(id, pkg, caller string, nodeType NodeType, layer GraphLayer, extra map[string]string) {
	if _, ok := g.nodes[id]; !ok {
		g.addNode(GraphNode{
			ID: id, Type: nodeType, Layer: layer,
			Label: scopedLabel(pkg, caller), Package: pkg,
			Metadata: mergeMetadata(callerMetadata(caller), extra),
		})
	}
}

func fieldForInterface(iface string, fields map[string]serviceField) string {
	for name, sf := range fields {
		if !isCapabilityField(name) {
			continue
		}
		if sf.Interface == iface || shortBeeType(sf.Interface) == shortBeeType(iface) {
			return name
		}
	}
	return ""
}

func (g *graphBuilder) attachWiring(capID, ifaceKey, capScope string) {
	entry, ok := g.wiring.lookup(ifaceKey)
	if !ok {
		return
	}
	g.attachWiringFromEntry(capID, entry, capScope)
}

func (g *graphBuilder) attachWiringFromEntry(fromID string, entry wiringEntry, capScope string) {
	prev := fromID
	for _, pkg := range entry.Stack {
		id := scopedNodeID(capScope, "wiring", pkg)
		meta := mergeMetadata(callerMetadata(capScope), map[string]string{
			"constructor": shortBeeSymbol(entry.Constructor),
		})
		g.ensureScopedPackageNode(id, pkg, capScope, NodeImplementation, LayerWiring, meta)
		g.linkEdge(prev, id, "resolves", "")
		prev = id
		g.pkgSeen[pkg] = struct{}{}
	}
	if entry.ImplPackage != "" && len(entry.Stack) == 0 {
		id := scopedNodeID(capScope, "wiring", entry.ImplPackage)
		meta := mergeMetadata(callerMetadata(capScope), map[string]string{
			"constructor": shortBeeSymbol(entry.Constructor),
		})
		g.ensureScopedPackageNode(id, entry.ImplPackage, capScope, NodeImplementation, LayerWiring, meta)
		g.linkEdge(prev, id, "resolves", "")
		prev = id
		g.pkgSeen[entry.ImplPackage] = struct{}{}
	}
	if entry.Backend != "" {
		bid := scopedNodeID(capScope, "backend", entry.Backend)
		g.addNode(GraphNode{
			ID: bid, Type: NodeBackend, Layer: LayerWiring,
			Label:    scopedLabel(entry.Backend, capScope),
			Metadata: callerMetadata(capScope),
		})
		g.linkEdge(prev, bid, "uses_backend", "")
	}
}

func (g *graphBuilder) hasEdgeBetween(from, to string) bool {
	for _, e := range g.edges {
		if e.From == from && e.To == to {
			return true
		}
	}
	return false
}

func (g *graphBuilder) hasOutgoingKind(from, kind string) bool {
	for _, e := range g.edges {
		if e.From == from && e.Kind == kind {
			return true
		}
	}
	return false
}

func (g *graphBuilder) addNode(n GraphNode) {
	if _, ok := g.nodes[n.ID]; !ok {
		g.nodes[n.ID] = n
	}
}

func (g *graphBuilder) linkEdge(from, to, kind, branch string) {
	key := from + "\x00" + to + "\x00" + kind + "\x00" + branch
	if _, ok := g.edgeSet[key]; ok {
		return
	}
	g.edgeSet[key] = struct{}{}
	g.edges = append(g.edges, GraphEdge{From: from, To: to, Kind: kind, Branch: branch})
}

func (g *graphBuilder) result() *EndpointGraph {
	nodes, edges := g.sortedGraphParts()
	return &EndpointGraph{
		Endpoint:           g.ep,
		Nodes:              nodes,
		Edges:              edges,
		CapabilityPackages: sortedKeys(g.pkgSeen),
		DirectCalls:        g.directCalls,
		CompressPackages:   g.compressPackages,
		EntityProjection:   buildEntityProjection(g.entityEvents, g.fields, g.wiring, false, ""),
	}
}

func (g *graphBuilder) programResult() *ProgramGraph {
	nodes, edges := g.sortedGraphParts()
	static, protocol := g.programPackageLists()
	return &ProgramGraph{
		Entry:              g.entry,
		Nodes:              nodes,
		Edges:              edges,
		CapabilityPackages: sortedKeys(g.pkgSeen),
		StaticPackages:     static,
		ProtocolPackages:   protocol,
		DirectCalls:        g.directCalls,
		CompressPackages:   g.compressPackages,
		EntityProjection:   buildEntityProjection(g.entityEvents, g.fields, g.wiring, true, g.entry.Package),
	}
}

func (g *graphBuilder) programPackageLists() (static, protocol []string) {
	for pkg := range g.pkgSeen {
		if !strings.HasPrefix(pkg, "pkg/") {
			continue
		}
		if _, ok := g.capPkgSeen[pkg]; !ok {
			static = append(static, pkg)
		}
	}
	static = sortedKeys(sliceSet(static))
	protocol = protocolPackagesPresent(g.pkgSeen)
	return static, protocol
}

func (g *graphBuilder) sortedGraphParts() ([]GraphNode, []GraphEdge) {
	nodes := make([]GraphNode, 0, len(g.nodes))
	for _, n := range g.nodes {
		nodes = append(nodes, n)
	}
	sortGraphNodes(nodes)
	sortGraphEdges(g.edges)
	return nodes, g.edges
}

func inferOp(method string) string {
	switch {
	case strings.HasPrefix(method, "Get"), strings.HasPrefix(method, "Has"),
		strings.HasPrefix(method, "Balance"), strings.HasPrefix(method, "Lookup"),
		strings.HasPrefix(method, "List"), strings.HasPrefix(method, "Peer"),
		strings.HasPrefix(method, "Iterate"), method == "Balances", method == "CompensatedBalances":
		return "read"
	case strings.HasPrefix(method, "Put"), strings.HasPrefix(method, "Delete"),
		strings.HasPrefix(method, "Set"), strings.HasPrefix(method, "Create"),
		strings.HasPrefix(method, "Upload"), strings.HasPrefix(method, "Deposit"),
		strings.HasPrefix(method, "Withdraw"), strings.HasPrefix(method, "Send"):
		return "write"
	default:
		return ""
	}
}

func packageFromInterface(iface string) string {
	s := shortBeeType(iface)
	if i := strings.Index(s, "."); i >= 0 {
		return "pkg/" + s[:i]
	}
	return ""
}

func sortEntryFunctions(roots []*ssa.Function) {
	sort.Slice(roots, func(i, j int) bool {
		return fnCallRef(roots[i]) < fnCallRef(roots[j])
	})
}
