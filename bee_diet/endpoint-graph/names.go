// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "strings"

// shortBeePkg strips the module prefix from a package path.
// github.com/ethersphere/bee/v2/pkg/accounting → pkg/accounting
func shortBeePkg(path string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, beeModule+"/") {
		return strings.TrimPrefix(path, beeModule+"/")
	}
	return path
}

// shortBeeType shortens a qualified bee type/interface name.
// github.com/ethersphere/bee/v2/pkg/accounting.Interface → accounting.Interface
func shortBeeType(typeName string) string {
	if typeName == "" {
		return ""
	}
	s := strings.TrimPrefix(typeName, beeModule+"/pkg/")
	if s == typeName {
		s = strings.TrimPrefix(typeName, beeModule+"/")
	}
	return s
}

// shortBeeSymbol shortens constructor/symbol references from node wiring.
func shortBeeSymbol(qualified string) string {
	if qualified == "" {
		return ""
	}
	return shortBeePkg(strings.TrimPrefix(qualified, beeModule+"/"))
}

const graphFileBase = "graph"

func pkgNodeID(pkg string) string {
	return "pkg:" + pkg
}
