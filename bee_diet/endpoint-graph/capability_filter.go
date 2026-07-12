// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "strings"

// capabilityFieldNames is populated from resolved api.Service fields.
var capabilityFieldNames = map[string]struct{}{}

func isCapabilityField(name string) bool {
	_, ok := capabilityFieldNames[name]
	return ok
}

// isBeeProjectInterface is true for interfaces defined under github.com/ethersphere/bee/v2/pkg/.
func isBeeProjectInterface(ifaceKey string) bool {
	return strings.HasPrefix(ifaceKey, beeModule+"/pkg/")
}

// isSubsystemCapabilityInterface excludes utility bee packages (log, jsonhttp, …) from
// Interface.Method capability nodes while still allowing them as pkg/* call targets.
func isSubsystemCapabilityInterface(ifaceKey string) bool {
	if !isBeeProjectInterface(ifaceKey) {
		return false
	}
	pkg := packageFromInterface(ifaceKey)
	if pkg == "" || isUtilityPackage(pkg) {
		return false
	}
	for _, frag := range skipPkgFragments {
		if strings.Contains(pkg, frag) {
			return false
		}
	}
	return true
}

func (g *graphBuilder) shouldRecordInvoke(ifaceKey, fieldName string) bool {
	if !isSubsystemCapabilityInterface(ifaceKey) {
		return false
	}
	if strings.HasSuffix(ifaceKey, "storage.Store") {
		return false
	}
	if g.programMode {
		return true
	}
	if fieldName != "" {
		return isCapabilityField(fieldName)
	}
	entry, ok := g.wiring.lookup(ifaceKey)
	if !ok {
		return false
	}
	_ = entry
	return true
}
