// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "strings"

// protocolChunkPackages lists Swarm chunk delivery/sync packages reached from
// node bootstrap mostly via static constructors (not interface invoke).
var protocolChunkPackages = []string{
	"pkg/pushsync",
	"pkg/pullsync",
	"pkg/retrieval",
	"pkg/pusher",
}

func protocolChunkRoot(pkg string) string {
	for _, root := range protocolChunkPackages {
		if pkg == root || strings.HasPrefix(pkg, root+"/") {
			return root
		}
	}
	return ""
}

func isProtocolChunkPackage(pkg string) bool {
	return protocolChunkRoot(pkg) != ""
}

func protocolPackagesPresent(pkgs map[string]struct{}) []string {
	var out []string
	for _, root := range protocolChunkPackages {
		for pkg := range pkgs {
			if pkg == root || strings.HasPrefix(pkg, root+"/") {
				out = append(out, root)
				break
			}
		}
	}
	return sortedKeys(sliceSet(out))
}

func sliceSet(ss []string) map[string]struct{} {
	m := make(map[string]struct{}, len(ss))
	for _, s := range ss {
		m[s] = struct{}{}
	}
	return m
}
