// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

// wiringEntry maps an interface to its production implementation package and
// optional linear runtime stack (outer → inner).
type wiringEntry struct {
	Interface   string
	ImplPackage string
	Constructor string
	Stack       []string
	Backend     string
}

var productionWiring = []wiringEntry{
	{
		Interface:   beeModule + "/pkg/storage.StateStorer",
		ImplPackage: "pkg/statestore/storeadapter",
		Constructor: beeModule + "/pkg/node.InitStateStore",
		Stack:       []string{"pkg/statestore/storeadapter", "pkg/storage/cache", "pkg/storage/leveldbstore"},
		Backend:     "disk:statestore",
	},
	{
		Interface:   beeModule + "/pkg/storage.StateStorerManager",
		ImplPackage: "pkg/statestore/storeadapter",
		Constructor: beeModule + "/pkg/node.InitStateStore",
		Stack:       []string{"pkg/statestore/storeadapter", "pkg/storage/cache", "pkg/storage/leveldbstore"},
		Backend:     "disk:statestore",
	},
	{
		Interface:   beeModule + "/pkg/storage.Store",
		ImplPackage: "pkg/storage/leveldbstore",
		Constructor: beeModule + "/pkg/node.InitStamperStore",
		Stack:       []string{"pkg/storage/leveldbstore"},
		Backend:     "disk:stamperstore",
	},
	{
		Interface:   beeModule + "/pkg/storage.IndexStore",
		ImplPackage: "pkg/storage/cache",
		Constructor: beeModule + "/pkg/node.InitStateStore",
		Stack:       []string{"pkg/storage/cache", "pkg/storage/leveldbstore"},
		Backend:     "disk:statestore",
	},
	{
		Interface:   beeModule + "/pkg/accounting.Interface",
		ImplPackage: "pkg/accounting",
		Constructor: beeModule + "/pkg/accounting.NewAccounting",
	},
	{
		Interface:   beeModule + "/pkg/api.Storer",
		ImplPackage: "pkg/storer",
		Constructor: beeModule + "/pkg/storer.New",
		Backend:     "disk:localstore",
	},
	{
		Interface:   beeModule + "/pkg/postage.Service",
		ImplPackage: "pkg/postage",
		Constructor: beeModule + "/pkg/postage.NewService",
	},
	{
		Interface:   beeModule + "/pkg/postage.Storer",
		ImplPackage: "pkg/postage/batchstore",
		Constructor: beeModule + "/pkg/node.NewBatchStore",
		Backend:     "disk:batchstore",
	},
	{
		Interface:   beeModule + "/pkg/p2p.DebugService",
		ImplPackage: "pkg/p2p/libp2p",
		Constructor: beeModule + "/pkg/p2p/libp2p.New",
		Backend:     "network:libp2p",
	},
	{
		Interface:   beeModule + "/pkg/topology.Driver",
		ImplPackage: "pkg/topology/kademlia",
		Constructor: beeModule + "/pkg/topology/kademlia.New",
	},
	{
		Interface:   beeModule + "/pkg/settlement/swap.Interface",
		ImplPackage: "pkg/settlement/swap",
		Constructor: beeModule + "/pkg/node.InitSwap",
		Backend:     "chain:swap",
	},
	{
		Interface:   beeModule + "/pkg/settlement/swap/chequebook.Service",
		ImplPackage: "pkg/settlement/swap/chequebook",
		Constructor: beeModule + "/pkg/node.InitChequebookService",
		Backend:     "chain:chequebook",
	},
	{
		Interface:   beeModule + "/pkg/transaction.Service",
		ImplPackage: "pkg/transaction",
		Constructor: beeModule + "/pkg/node.InitChain",
		Backend:     "chain:rpc",
	},
	{
		Interface:   beeModule + "/pkg/pingpong.Interface",
		ImplPackage: "pkg/pingpong",
		Constructor: beeModule + "/pkg/pingpong.New",
	},
	{
		Interface:   beeModule + "/pkg/settlement.Interface",
		ImplPackage: "pkg/settlement/pseudosettle",
		Constructor: beeModule + "/pkg/settlement/pseudosettle.New",
	},
}

type wiringIndex struct {
	byIface       map[string]wiringEntry
	byConstructor map[string]wiringEntry
}

func newWiringIndex() *wiringIndex {
	idx := &wiringIndex{
		byIface:       make(map[string]wiringEntry),
		byConstructor: make(map[string]wiringEntry),
	}
	for _, e := range productionWiring {
		idx.byIface[e.Interface] = e
		if e.Constructor != "" {
			idx.byConstructor[shortBeeSymbol(e.Constructor)] = e
		}
	}
	return idx
}

func (w *wiringIndex) lookup(iface string) (wiringEntry, bool) {
	if w == nil {
		return wiringEntry{}, false
	}
	e, ok := w.byIface[iface]
	return e, ok
}

func (w *wiringIndex) implPackage(iface string) string {
	e, ok := w.lookup(iface)
	if !ok {
		return ""
	}
	return e.ImplPackage
}

func (w *wiringIndex) lookupConstructor(ref string) (wiringEntry, bool) {
	if w == nil || ref == "" {
		return wiringEntry{}, false
	}
	e, ok := w.byConstructor[ref]
	return e, ok
}
