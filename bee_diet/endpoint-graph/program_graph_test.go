// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"strings"
	"testing"
)

func TestParseEntrySpecNode(t *testing.T) {
	root := beeRoot(t)
	spec, err := parseEntrySpec("node", root)
	if err != nil {
		t.Fatal(err)
	}
	if spec.PkgRel != "pkg/node" || spec.FuncName != "NewBee" {
		t.Fatalf("spec: %+v", spec)
	}
}

func TestParseEntrySpecMain(t *testing.T) {
	root := beeRoot(t)
	spec, err := parseEntrySpec("main", root)
	if err != nil {
		t.Fatal(err)
	}
	if spec.PkgRel != "cmd/bee" || spec.FuncName != "main" {
		t.Fatalf("spec: %+v", spec)
	}
}

func TestWiringConstructorLookup(t *testing.T) {
	idx := newWiringIndex()
	for _, ref := range []string{"pkg/storer.New", "pkg/p2p/libp2p.New", "pkg/node.InitSwap"} {
		if _, ok := idx.lookupConstructor(ref); !ok {
			t.Fatalf("missing constructor wiring for %s", ref)
		}
	}
	e, ok := idx.lookupConstructor("pkg/storer.New")
	if !ok || e.Backend != "disk:localstore" {
		t.Fatalf("storer.New wiring: %+v ok=%v", e, ok)
	}
}

func TestNodeGraphDirectCallsFromNewBee(t *testing.T) {
	root := beeRoot(t)
	prog, err := loadSSAForEntry(root, entrySpec{PkgRel: "pkg/node", PkgPath: beeModule + "/pkg/node"})
	if err != nil {
		t.Fatal(err)
	}
	fields, err := loadServiceFields(root)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := parseEntrySpec("node", root)
	if err != nil {
		t.Fatal(err)
	}
	g, err := analyzeProgramGraph(prog, spec, fields, 0, true, false)
	if err != nil {
		t.Fatal(err)
	}

	for _, n := range g.Nodes {
		if n.Type == NodeExternalDeps {
			t.Fatalf("program graph must not include external deps: %+v", n)
		}
	}

	backendLabels := map[string]struct{}{}
	for _, n := range g.Nodes {
		if n.Type == NodeBackend {
			backendLabels[strings.Split(n.Label, " ←")[0]] = struct{}{}
		}
	}
	for _, want := range []string{"disk:localstore", "disk:statestore", "disk:batchstore", "network:libp2p", "chain:rpc", "chain:swap", "chain:chequebook"} {
		if _, ok := backendLabels[want]; !ok {
			t.Fatalf("missing backend %s; have %v", want, sortedKeys(backendLabels))
		}
	}

	fromEntryInvokes := 0
	fromEntryCalls := 0
	for _, e := range g.Edges {
		if e.From != "entry:NewBee" {
			continue
		}
		switch e.Kind {
		case "invokes":
			fromEntryInvokes++
		case "calls":
			fromEntryCalls++
		}
	}
	if fromEntryCalls == 0 {
		t.Fatal("expected calls chain from entry via function nodes")
	}
	if fromEntryInvokes >= 141 {
		t.Fatalf("too many flat invokes from entry (%d)", fromEntryInvokes)
	}

	for _, pkg := range []string{"pkg/pullsync", "pkg/retrieval", "pkg/pushsync", "pkg/pusher"} {
		if !containsString(g.ProtocolPackages, pkg) {
			t.Fatalf("missing %s in protocol_packages: %v", pkg, g.ProtocolPackages)
		}
	}

	hasStaticReach := false
	for _, e := range g.Edges {
		if e.Kind == "static_reach" {
			hasStaticReach = true
			break
		}
	}
	if !hasStaticReach {
		t.Fatal("missing static_reach edges for protocol packages")
	}

	wantCaps := []string{
		"postage.Storer.GetChainState",
		"pss.Interface.SetPushSyncer",
		"chequebook.Service.Address",
		"transaction.Backend.ChainID",
	}
	foundCaps := map[string]bool{}
	for _, n := range g.Nodes {
		if n.Type != NodeCapability {
			continue
		}
		key := n.Interface + "." + n.Method
		for _, w := range wantCaps {
			if strings.Contains(key, w) || strings.Contains(n.Label, w) {
				foundCaps[w] = true
			}
		}
	}
	for _, w := range wantCaps {
		if !foundCaps[w] {
			t.Fatalf("missing capability matching %q", w)
		}
	}
}

func TestProgramGraphDirectCallsFromMain(t *testing.T) {
	root := beeRoot(t)
	prog, err := loadSSAProgram(root)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := loadServiceFields(root)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := parseEntrySpec("main", root)
	if err != nil {
		t.Fatal(err)
	}
	g, err := analyzeProgramGraph(prog, spec, fields, 8, true, false)
	if err != nil {
		t.Fatal(err)
	}

	hasEntry := false
	for _, n := range g.Nodes {
		if n.Type == NodeEntry {
			hasEntry = true
		}
		if n.Type == NodeExternalDeps {
			t.Fatalf("program graph must not include external deps: %+v", n)
		}
	}
	if !hasEntry {
		t.Fatal("missing entry node")
	}
}
