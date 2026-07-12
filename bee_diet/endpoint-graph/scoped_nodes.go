// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "strings"

const apiHandlerPkg = "pkg/api"

// omittedGraphMiddleware lists passthrough middleware excluded from dependency graphs.
// They do not participate in SSA analysis and only add noise on merged graphs.
var omittedGraphMiddleware = map[string]struct{}{
	// feature gates
	"checkRouteAvailability":             {},
	"checkSwapAvailability":              {},
	"checkChequebookAvailability":        {},
	"checkChainAvailability":             {},
	"checkStorageIncentivesAvailability": {},
	// metrics / tracing / logging / limits
	"contentLengthMetricMiddleware":              {},
	"downloadSpeedMetricMiddleware":              {},
	"newTracingHandler":                          {},
	"httpaccess.NewHTTPAccessSuppressLogHandler": {},
	"jsonhttp.NewMaxBodyBytesHandler":            {},
}

// scopedNodeID identifies a node by caller context + kind + target.
func scopedNodeID(caller, kind, target string) string {
	return "n:" + sanitizeScopePart(caller) + ":" + kind + ":" + sanitizeScopePart(target)
}

// scopedLabel renders "target ← caller" for graph readability.
func scopedLabel(target, caller string) string {
	return target + " ← " + caller
}

func handlerCallRef(handler string) string {
	if handler == "" || handler == "anonymous" {
		return apiHandlerPkg + ".anonymous"
	}
	return apiHandlerPkg + "." + handler
}

func capabilityTarget(shortIface, method string) string {
	if i := strings.Index(shortIface, "."); i >= 0 {
		return "pkg/" + shortIface[:i] + "." + shortIface[i+1:] + "." + method
	}
	return shortIface + "." + method
}

// entityTarget is the package-qualified interface/type without a method suffix.
// api.Storer → pkg/api.Storer
func entityTarget(shortIface string) string {
	if i := strings.Index(shortIface, "."); i >= 0 {
		return "pkg/" + shortIface[:i] + "." + shortIface[i+1:]
	}
	return shortIface
}

func serviceFieldTarget(sf serviceField) string {
	if sf.Package != "" {
		return sf.Package + ".s." + sf.Field
	}
	return "s." + sf.Field
}

func sanitizeScopePart(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '.', r == '_', r == '-', r == '/':
			return r
		default:
			return '_'
		}
	}, s)
	if s == "" {
		return "anon"
	}
	return s
}

func callerMetadata(caller string) map[string]string {
	if caller == "" {
		return nil
	}
	return map[string]string{"caller": caller}
}

func mergeMetadata(base map[string]string, extra map[string]string) map[string]string {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func isOmittedGraphMiddleware(name string) bool {
	_, ok := omittedGraphMiddleware[name]
	return ok
}
