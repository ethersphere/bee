// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"strings"

	"golang.org/x/tools/go/ssa"
)

// isExternalHandler reports handlers implemented outside bee pkg/api (stdlib / third-party).
func isExternalHandler(name string) bool {
	switch name {
	case "promhttp", "expvar":
		return true
	}
	if strings.HasPrefix(name, "pprof.") {
		return true
	}
	return strings.HasPrefix(name, "fgprof.") || strings.HasPrefix(name, "expvar.")
}

func externalPkgPath(fn *ssa.Function) string {
	if fn == nil || fn.Pkg == nil || fn.Pkg.Pkg == nil {
		return ""
	}
	path := fn.Pkg.Pkg.Path()
	if path == "" || strings.HasPrefix(path, beeModule) {
		return ""
	}
	return path
}
