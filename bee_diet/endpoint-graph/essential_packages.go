// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "strings"

// essentialPackages lists bee packages classified as essential (see document-summary.md).
var essentialPackages = []string{
	// storage
	"pkg/storer",
	"pkg/statestore/storeadapter",
	"pkg/postage",
	"pkg/storage/leveldbstore",
	"pkg/sharky",
	"pkg/shed",
	// network
	"pkg/p2p/libp2p",
	"pkg/hive",
	"pkg/pingpong",
	"pkg/skipper",
	"pkg/bzz",
	// routing
	"pkg/topology/kademlia",
	"pkg/stabilization",
	"pkg/discovery",
	// chain
	"pkg/transaction",
	"pkg/settlement/swap",
	"pkg/settlement/swap/chequebook",
	"pkg/settlement/swap/erc20",
	"pkg/postage/postagecontract",
	"pkg/postage/listener",
	// protocol-chunks
	"pkg/pushsync",
	"pkg/pullsync",
	"pkg/retrieval",
	"pkg/pusher",
	"pkg/puller",
	// economics
	"pkg/accounting",
	"pkg/settlement/pseudosettle",
	// incentives
	"pkg/storageincentives",
	"pkg/storageincentives/redistribution",
	"pkg/storageincentives/staking",
	// operator
	"pkg/status",
	"pkg/salud",
}

func isEssentialPackage(pkg string) bool {
	if pkg == "" {
		return false
	}
	for _, essential := range essentialPackages {
		if pkg == essential || strings.HasPrefix(pkg, essential+"/") {
			return true
		}
	}
	return false
}

func yedFillColorForNode(n GraphNode, essentialColor bool) string {
	if essentialColor && isEssentialPackage(n.Package) {
		return "#FF6666"
	}
	return yedFillColor(n.Layer)
}
