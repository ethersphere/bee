// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/ssa"
)

const (
	entityKindDependsOn = "depends_on"
)

type entityEvent struct {
	callerScope string
	fieldName   string
	ifaceKey    string
	method      string
	branch      string
	impls       []*ssa.Function
	ambiguous   bool
	implCount   int
}

func entityRuntimeID(scope string) string {
	if strings.HasPrefix(scope, "pkg/node.") || scope == "pkg/node.NewBee" {
		return "entity:runtime:pkg/node"
	}
	return "entity:runtime:api"
}

func entityFieldID(field string) string {
	return "entity:field:" + field
}

func entityImplID(pkg, typeName string) string {
	if typeName == "" {
		return "entity:impl:" + pkg
	}
	return "entity:impl:" + pkg + "." + typeName
}

func entityIfaceID(ifaceKey string) string {
	return "entity:iface:" + shortBeeType(ifaceKey)
}

func implEntityFromFunction(fn *ssa.Function) (string, bool) {
	if fn == nil || fn.Signature.Recv() == nil {
		return "", false
	}
	pkg, ok := beePkgPath(fn)
	if !ok {
		return "", false
	}
	recv := fn.Signature.Recv().Type()
	recv = types.Unalias(recv)
	if ptr, ok := recv.(*types.Pointer); ok {
		recv = ptr.Elem()
	}
	named, ok := recv.(*types.Named)
	if !ok {
		return "", false
	}
	return entityImplID(pkg, named.Obj().Name()), true
}

func (g *graphBuilder) recordEntityInvoke(call ssa.CallInstruction, caller *ssa.Function, fieldName, ifaceKey, branch string, res invokeResolution) {
	method := call.Common().Method
	if method == nil {
		return
	}
	g.entityEvents = append(g.entityEvents, entityEvent{
		callerScope: g.callScope,
		fieldName:   fieldName,
		ifaceKey:    ifaceKey,
		method:      method.Name(),
		branch:      branch,
		impls:       res.impls,
		ambiguous:   res.ambiguous,
		implCount:   res.implCount,
	})
}

func buildEntityProjection(events []entityEvent, fields map[string]serviceField, wiring *wiringIndex, programMode bool, entryPkg string) *EntityProjection {
	rootID := "entity:runtime:api"
	rootLabel := apiHandlerPkg
	if programMode && entryPkg != "" {
		rootID = "entity:runtime:" + entryPkg
		rootLabel = entryPkg
	}

	nodes := map[string]EntityRef{
		rootID: {ID: rootID, Label: rootLabel, Package: rootLabel},
	}
	edgeAcc := map[string]*EntityDep{}
	unresolved := []EntityUnresolved{}

	ensureNode := func(ref EntityRef) {
		if _, ok := nodes[ref.ID]; ok {
			return
		}
		nodes[ref.ID] = ref
	}
	addDep := func(from, to, method, branch string, meta map[string]string) {
		if from == "" || to == "" || from == to {
			return
		}
		key := from + "\x00" + to + "\x00" + branch
		dep, ok := edgeAcc[key]
		if !ok {
			dep = &EntityDep{From: from, To: to, Kind: entityKindDependsOn, Branch: branch, Metadata: meta}
			edgeAcc[key] = dep
		}
		if method != "" && !containsString(dep.Methods, method) {
			dep.Methods = append(dep.Methods, method)
		}
	}

	for _, ev := range events {
		callerID := callerEntityFromScope(ev.callerScope, fields, programMode, entryPkg)
		calleeID, calleeRef, reason, count := resolveCalleeEntity(ev, fields, wiring)
		if calleeRef.ID != "" {
			ensureNode(calleeRef)
		}
		if callerID != "" && calleeID != "" {
			meta := map[string]string{}
			if reason != "" {
				meta["resolution"] = reason
			}
			addDep(callerID, calleeID, ev.method, ev.branch, meta)
		}
		if reason == "ambiguous" || reason == "no_impl" || reason == "no_wiring" {
			unresolved = append(unresolved, EntityUnresolved{
				Site:   ev.callerScope,
				Iface:  shortBeeType(ev.ifaceKey),
				Method: ev.method,
				Reason: reason,
				Count:  count,
			})
		}
	}

	nodeList := make([]EntityRef, 0, len(nodes))
	for _, n := range nodes {
		if len(n.Methods) > 1 {
			sort.Strings(n.Methods)
		}
		nodeList = append(nodeList, n)
	}
	sort.Slice(nodeList, func(i, j int) bool { return nodeList[i].ID < nodeList[j].ID })

	edgeList := make([]EntityDep, 0, len(edgeAcc))
	for _, dep := range edgeAcc {
		if len(dep.Methods) > 1 {
			sort.Strings(dep.Methods)
		}
		edgeList = append(edgeList, *dep)
	}
	sort.Slice(edgeList, func(i, j int) bool {
		if edgeList[i].From != edgeList[j].From {
			return edgeList[i].From < edgeList[j].From
		}
		return edgeList[i].To < edgeList[j].To
	})

	sort.Slice(unresolved, func(i, j int) bool {
		if unresolved[i].Site != unresolved[j].Site {
			return unresolved[i].Site < unresolved[j].Site
		}
		return unresolved[i].Method < unresolved[j].Method
	})

	return &EntityProjection{Nodes: nodeList, Edges: edgeList, Unresolved: unresolved}
}

func callerEntityFromScope(scope string, fields map[string]serviceField, programMode bool, entryPkg string) string {
	if programMode {
		if entryPkg != "" {
			return "entity:runtime:" + entryPkg
		}
	}
	if strings.HasPrefix(scope, apiHandlerPkg+".") || strings.HasPrefix(scope, "handler:") {
		return "entity:runtime:api"
	}
	for name, sf := range fields {
		shortIface := shortBeeType(sf.Interface)
		if shortIface != "" && strings.Contains(scope, shortIface) {
			return entityFieldID(name)
		}
		if sf.Package != "" && strings.Contains(scope, sf.Package) {
			return entityFieldID(name)
		}
	}
	return entityRuntimeID(scope)
}

func resolveCalleeEntity(ev entityEvent, fields map[string]serviceField, wiring *wiringIndex) (id string, ref EntityRef, reason string, count int) {
	ifaceKey := ev.ifaceKey
	method := ev.method

	if ev.fieldName != "" {
		sf, ok := fields[ev.fieldName]
		if ok {
			if entry, wired := wiring.lookup(sf.Interface); wired && entry.ImplPackage != "" {
				id = entityFieldID(ev.fieldName)
				ref = EntityRef{
					ID: id, Label: "s." + ev.fieldName, Field: ev.fieldName,
					Interface: shortBeeType(sf.Interface), Package: sf.Package,
					Impl: entry.ImplPackage, Methods: []string{method},
					Metadata: map[string]string{"resolution": "wiring"},
				}
				return id, ref, "wiring", 0
			}
			id = entityFieldID(ev.fieldName)
			ref = EntityRef{
				ID: id, Label: "s." + ev.fieldName, Field: ev.fieldName,
				Interface: shortBeeType(sf.Interface), Package: sf.Package,
				Methods: []string{method},
			}
			if len(ev.impls) == 1 {
				if implID, ok := implEntityFromFunction(ev.impls[0]); ok {
					ref.Impl = strings.TrimPrefix(implID, "entity:impl:")
					ref.Metadata = map[string]string{"resolution": "ssa_recv"}
				}
			}
			return id, ref, "", 0
		}
	}

	if entry, ok := wiring.lookup(ifaceKey); ok && entry.ImplPackage != "" {
		id = entityImplID(entry.ImplPackage, "")
		ref = EntityRef{
			ID: id, Label: entry.ImplPackage, Package: entry.ImplPackage,
			Interface: shortBeeType(ifaceKey), Impl: entry.ImplPackage,
			Methods: []string{method}, Metadata: map[string]string{"resolution": "wiring"},
		}
		return id, ref, "wiring", 0
	}

	if len(ev.impls) == 1 {
		if implID, ok := implEntityFromFunction(ev.impls[0]); ok {
			ref = EntityRef{
				ID: implID, Label: strings.TrimPrefix(implID, "entity:impl:"),
				Methods: []string{method}, Metadata: map[string]string{"resolution": "ssa_recv"},
			}
			if pkg, ok := beePkgPath(ev.impls[0]); ok {
				ref.Package = pkg
			}
			return implID, ref, "ssa_recv", 0
		}
	}

	if ev.ambiguous {
		id = entityIfaceID(ifaceKey)
		ref = EntityRef{
			ID: id, Label: shortBeeType(ifaceKey), Interface: shortBeeType(ifaceKey),
			Methods: []string{method}, Metadata: map[string]string{"resolution": "ambiguous"},
		}
		return id, ref, "ambiguous", ev.implCount
	}
	if len(ev.impls) == 0 {
		if _, ok := wiring.lookup(ifaceKey); !ok {
			id = entityIfaceID(ifaceKey)
			ref = EntityRef{
				ID: id, Label: shortBeeType(ifaceKey), Interface: shortBeeType(ifaceKey),
				Methods: []string{method}, Metadata: map[string]string{"resolution": "iface_only"},
			}
			return id, ref, "no_impl", 0
		}
	}

	id = entityIfaceID(ifaceKey)
	ref = EntityRef{
		ID: id, Label: shortBeeType(ifaceKey), Interface: shortBeeType(ifaceKey),
		Methods: []string{method}, Metadata: map[string]string{"resolution": "iface_only"},
	}
	return id, ref, "iface_only", 0
}

func mergeEntityProjections(graphs []loadedEndpointGraph) *EntityProjection {
	if len(graphs) == 0 {
		return nil
	}
	nodes := map[string]EntityRef{}
	edgeAcc := map[string]*EntityDep{}
	var unresolved []EntityUnresolved

	for _, lg := range graphs {
		if lg.graph.EntityProjection == nil {
			continue
		}
		ep := lg.graph.EntityProjection
		for _, n := range ep.Nodes {
			existing, ok := nodes[n.ID]
			if !ok {
				nodes[n.ID] = n
				continue
			}
			existing.Methods = mergeStringLists(existing.Methods, n.Methods)
			nodes[n.ID] = existing
		}
		for _, e := range ep.Edges {
			key := e.From + "\x00" + e.To + "\x00" + e.Branch
			dep, ok := edgeAcc[key]
			if !ok {
				copy := e
				edgeAcc[key] = &copy
				continue
			}
			dep.Methods = mergeStringLists(dep.Methods, e.Methods)
		}
		unresolved = append(unresolved, ep.Unresolved...)
	}
	if len(nodes) == 0 {
		return nil
	}

	nodeList := make([]EntityRef, 0, len(nodes))
	for _, n := range nodes {
		if len(n.Methods) > 1 {
			sort.Strings(n.Methods)
		}
		nodeList = append(nodeList, n)
	}
	sort.Slice(nodeList, func(i, j int) bool { return nodeList[i].ID < nodeList[j].ID })

	edgeList := make([]EntityDep, 0, len(edgeAcc))
	for _, dep := range edgeAcc {
		if len(dep.Methods) > 1 {
			sort.Strings(dep.Methods)
		}
		edgeList = append(edgeList, *dep)
	}
	sort.Slice(edgeList, func(i, j int) bool {
		if edgeList[i].From != edgeList[j].From {
			return edgeList[i].From < edgeList[j].From
		}
		return edgeList[i].To < edgeList[j].To
	})

	return &EntityProjection{Nodes: nodeList, Edges: edgeList, Unresolved: unresolved}
}

func mergeStringLists(a, b []string) []string {
	seen := map[string]struct{}{}
	for _, s := range a {
		seen[s] = struct{}{}
	}
	for _, s := range b {
		if _, ok := seen[s]; !ok {
			a = append(a, s)
			seen[s] = struct{}{}
		}
	}
	sort.Strings(a)
	return a
}
