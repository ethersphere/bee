// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
)

const apiVersionPath = "/v1"

// implicitMountMiddleware is applied inside the local handle() closure.
var implicitMountMiddleware = map[string][]string{
	"mountAPI":           {"checkRouteAvailability"},
	"mountBusinessDebug": {"checkRouteAvailability"},
}

// ParseRouterAST extracts endpoints from pkg/api/router.go using go/ast.
func ParseRouterAST(filename string, src []byte) ([]Endpoint, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return nil, fmt.Errorf("parse router: %w", err)
	}

	var endpoints []Endpoint
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name == nil {
			continue
		}
		mount := mountFromFunc(fn.Name.Name)
		if mount == "" {
			continue
		}
		eps := extractMountEndpoints(mount, fn.Body)
		endpoints = append(endpoints, eps...)
	}

	endpoints = append(endpoints, supplementEndpointsAST()...)
	endpoints = dedupeEndpoints(endpoints)
	return endpoints, nil
}

func mountFromFunc(name string) string {
	switch name {
	case "mountTechnicalDebug":
		return "mountTechnicalDebug"
	case "mountBusinessDebug":
		return "mountBusinessDebug"
	case "mountAPI":
		return "mountAPI"
	default:
		return ""
	}
}

func extractMountEndpoints(mount string, body *ast.BlockStmt) []Endpoint {
	if body == nil {
		return nil
	}
	var (
		endpoints     []Endpoint
		usesHandle    bool
		subdomainPath string
	)
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		if _, ok := n.(*ast.IfStmt); ok {
			return true
		}
		assign, ok := n.(*ast.AssignStmt)
		if ok && len(assign.Lhs) == 1 {
			if id, ok := assign.Lhs[0].(*ast.Ident); ok && id.Name == "handle" {
				if _, ok := assign.Rhs[0].(*ast.FuncLit); ok {
					usesHandle = true
				}
			}
		}
		if assign, ok := n.(*ast.AssignStmt); ok && len(assign.Lhs) == 1 {
			if sel, ok := assign.Lhs[0].(*ast.SelectorExpr); ok {
				if x, ok := sel.X.(*ast.Ident); ok && x.Name == "subdomainRouter" && sel.Sel.Name == "Handle" {
					if lit := stringLit(assign.Rhs[0]); lit != "" {
						subdomainPath = "{subdomain}.swarm.localhost/" + strings.TrimPrefix(lit, "/")
					}
				}
			}
		}
		return true
	})

	var walk func(stmt ast.Stmt, cond bool)
	walk = func(stmt ast.Stmt, cond bool) {
		if stmt == nil {
			return
		}
		switch s := stmt.(type) {
		case *ast.BlockStmt:
			for _, st := range s.List {
				walk(st, cond)
			}
		case *ast.IfStmt:
			walk(s.Body, true)
			if s.Else != nil {
				walk(s.Else, false)
			}
		default:
			if ep := endpointFromStmt(mount, usesHandle, cond, subdomainPath, s); ep != nil {
				endpoints = append(endpoints, ep...)
			}
		}
	}
	for _, stmt := range body.List {
		walk(stmt, false)
	}
	return endpoints
}

func endpointFromStmt(mount string, usesHandle, conditional bool, subdomainPath string, stmt ast.Stmt) []Endpoint {
	exprStmt, ok := stmt.(*ast.ExprStmt)
	if !ok {
		return nil
	}
	call, ok := exprStmt.X.(*ast.CallExpr)
	if !ok {
		return nil
	}

	switch fun := call.Fun.(type) {
	case *ast.Ident:
		if fun.Name == "handle" && usesHandle && len(call.Args) >= 2 {
			return parseRouteRegistration(mount, usesHandle, conditional, stringLit(call.Args[0]), call.Args[1])
		}
	case *ast.SelectorExpr:
		if fun.Sel.Name == "Handler" {
			if pathCall, ok := fun.X.(*ast.CallExpr); ok {
				if pathMethod, ok := isRouterPathCall(pathCall.Fun); ok {
					path := stringLit(pathCall.Args[0])
					if pathMethod == "PathPrefix" {
						path = strings.TrimSuffix(path, "/") + "/*"
					}
					if path != "" && len(call.Args) >= 1 {
						return parseRouteRegistration(mount, false, conditional, path, call.Args[0])
					}
				}
			}
			if id, ok := fun.X.(*ast.Ident); ok && id.Name == "subdomainRouter" && subdomainPath != "" && len(call.Args) >= 1 {
				return parseRouteRegistration(mount, false, conditional, subdomainPath, call.Args[0])
			}
		}
		if isRouterHandleCall(fun) && len(call.Args) >= 2 {
			path := stringLit(call.Args[0])
			if path == "" {
				return nil
			}
			handlerExpr := call.Args[1]
			return parseRouteRegistration(mount, false, conditional, path, handlerExpr)
		}
	}
	return nil
}

func isRouterPathCall(fun ast.Expr) (method string, ok bool) {
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	if sel.Sel.Name != "Path" && sel.Sel.Name != "PathPrefix" {
		return "", false
	}
	recv, ok := sel.X.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	id, ok := recv.X.(*ast.Ident)
	if !ok || id.Name != "s" || recv.Sel.Name != "router" {
		return "", false
	}
	return sel.Sel.Name, true
}

func isRouterHandleCall(fun *ast.SelectorExpr) bool {
	if fun.Sel.Name != "Handle" && fun.Sel.Name != "HandleFunc" {
		return false
	}
	recv, ok := fun.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := recv.X.(*ast.Ident)
	return ok && id.Name == "s" && recv.Sel.Name == "router"
}

func parseRouteRegistration(mount string, usesHandle, conditional bool, path string, handlerExpr ast.Expr) []Endpoint {
	if path == "" {
		return nil
	}
	parsed := parseHandlerExpr(handlerExpr)
	if parsed.handler == "" {
		if h := handlerFromExpr(handlerExpr); h != "" {
			parsed.handler = h
			parsed.external = isExternalHandler(h)
		}
	}
	if parsed.handler == "" {
		parsed.handler = "unknown"
	} else if isExternalHandler(parsed.handler) {
		parsed.external = true
	}

	middlewares := append([]string{}, implicitMountMiddleware[mount]...)
	if usesHandle {
		_ = usesHandle
	}
	middlewares = append(middlewares, parsed.middlewares...)

	var out []Endpoint
	methods := parsed.methods
	if len(methods) == 0 {
		methods = map[string]string{"GET": parsed.handler}
	}
	for _, method := range sortedHTTPMethods(methods) {
		handler := methods[method]
		ep := Endpoint{
			Mount:       mount,
			Method:      method,
			Path:        path,
			Handler:     handler,
			Middlewares: uniqueStrings(middlewares),
			Conditional: conditional,
			External:    parsed.external,
		}
		if mount == "mountAPI" || mount == "mountBusinessDebug" {
			ep.PathV1 = apiVersionPath + path
		}
		out = append(out, ep)
	}
	return out
}

type parsedHandler struct {
	middlewares []string
	methods     map[string]string
	handler     string
	external    bool
}

func parseHandlerExpr(expr ast.Expr) parsedHandler {
	switch e := expr.(type) {
	case *ast.CallExpr:
		return parseHandlerCall(e)
	case *ast.CompositeLit:
		return parseMethodHandler(e)
	case *ast.SelectorExpr:
		name := selectorName(e)
		if e.Sel.Name == "Handler" || isExternalHandler(name) {
			return parsedHandler{handler: name, external: isExternalHandler(name)}
		}
		if h := handlerFromExpr(e); h != "" {
			return parsedHandler{handler: h, external: isExternalHandler(h)}
		}
	}
	return parsedHandler{}
}

func parseHandlerCall(call *ast.CallExpr) parsedHandler {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		switch fun.Sel.Name {
		case "ChainHandlers":
			return parseChainHandlers(call.Args)
		case "FinalHandler":
			if len(call.Args) == 1 {
				return parseHandlerExpr(call.Args[0])
			}
		case "FinalHandlerFunc":
			if len(call.Args) == 1 {
				return parseHandlerExpr(call.Args[0])
			}
		case "HandlerFunc":
			if len(call.Args) == 1 {
				h := handlerFromExpr(call.Args[0])
				return parsedHandler{handler: h}
			}
		case "InstrumentMetricHandler":
			if len(call.Args) == 2 {
				p := parseHandlerExpr(call.Args[1])
				p.external = true
				p.handler = "promhttp"
				return p
			}
		}
	case *ast.Ident:
		if fun.Name == "fgprof" && len(call.Args) == 0 {
			return parsedHandler{handler: "fgprof.Handler", external: true}
		}
	}
	return parsedHandler{}
}

func parseChainHandlers(args []ast.Expr) parsedHandler {
	var (
		middlewares []string
		terminal    parsedHandler
	)
	for _, arg := range args {
		if isFinalHandlerArg(arg) {
			terminal = parseHandlerExpr(unwrapFinal(arg))
			continue
		}
		if mw := middlewareFromExpr(arg); mw != "" {
			middlewares = append(middlewares, mw)
			continue
		}
		inner := parseHandlerExpr(arg)
		middlewares = append(middlewares, inner.middlewares...)
		if inner.handler != "" {
			terminal = inner
		}
	}
	terminal.middlewares = append(middlewares, terminal.middlewares...)
	if terminal.handler == "" && len(terminal.methods) == 0 {
		for i := len(args) - 1; i >= 0; i-- {
			if !isFinalHandlerArg(args[i]) {
				continue
			}
			if h := handlerFromExpr(unwrapFinal(args[i])); h != "" {
				terminal.handler = h
				terminal.external = isExternalHandler(h)
				break
			}
		}
	}
	return terminal
}

func isFinalHandlerArg(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return sel.Sel.Name == "FinalHandler" || sel.Sel.Name == "FinalHandlerFunc"
}

func unwrapFinal(expr ast.Expr) ast.Expr {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) == 0 {
		return expr
	}
	return call.Args[0]
}

func parseMethodHandler(lit *ast.CompositeLit) parsedHandler {
	var out parsedHandler
	out.methods = map[string]string{}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		method := stringLit(kv.Key)
		if method == "" {
			continue
		}
		inner := parseHandlerExpr(kv.Value)
		out.middlewares = append(out.middlewares, inner.middlewares...)
		h := inner.handler
		if h == "" {
			h = handlerFromExpr(kv.Value)
		}
		out.methods[method] = h
		if inner.external {
			out.external = true
		}
	}
	return out
}

func middlewareFromExpr(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "s" {
				return sel.Sel.Name
			}
		}
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			name := selectorName(sel)
			if strings.Contains(name, "HTTPAccess") || strings.Contains(name, "MaxBodyBytes") {
				return name
			}
		}
	case *ast.SelectorExpr:
		if id, ok := e.X.(*ast.Ident); ok && id.Name == "s" {
			return e.Sel.Name
		}
	}
	return ""
}

func handlerFromExpr(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		if id, ok := e.X.(*ast.Ident); ok && id.Name == "s" {
			return e.Sel.Name
		}
		return selectorName(e)
	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "HandlerFunc" && len(e.Args) == 1 {
			return handlerFromExpr(e.Args[0])
		}
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Handler" {
			name := selectorName(sel)
			if isExternalHandler(name) {
				return name
			}
		}
		p := parseHandlerExpr(e)
		return p.handler
	case *ast.FuncLit:
		return "anonymous"
	case *ast.Ident:
		if e.Name == "expvar" {
			return "expvar"
		}
	}
	return ""
}

func selectorName(sel *ast.SelectorExpr) string {
	switch x := sel.X.(type) {
	case *ast.Ident:
		return x.Name + "." + sel.Sel.Name
	case *ast.SelectorExpr:
		return selectorName(x) + "." + sel.Sel.Name
	default:
		return sel.Sel.Name
	}
}

func stringLit(expr ast.Expr) string {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	return strings.Trim(lit.Value, `"`)
}

func supplementEndpointsAST() []Endpoint {
	return []Endpoint{
		{Mount: "mountAPI", Method: "GET", Path: "{subdomain}.swarm.localhost/{path}", Handler: "subdomainHandler"},
	}
}

func dedupeEndpoints(in []Endpoint) []Endpoint {
	seen := map[string]struct{}{}
	var out []Endpoint
	for _, ep := range in {
		key := ep.Mount + "\x00" + ep.Method + "\x00" + ep.Path
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ep)
	}
	return out
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func EndpointSlug(ep Endpoint) string {
	path := endpointPathName(ep.Path)
	return ep.Mount + "__" + ep.Method + "__" + path
}

// EndpointDirName is the per-endpoint output folder: METHOD_endpointname (e.g. GET_accounting).
func EndpointDirName(ep Endpoint) string {
	return strings.ToUpper(ep.Method) + "_" + endpointPathName(ep.Path)
}

func endpointPathName(path string) string {
	path = strings.Trim(path, "/")
	path = strings.ReplaceAll(path, "/", "_")
	path = strings.ReplaceAll(path, "{", "")
	path = strings.ReplaceAll(path, "}", "")
	path = strings.ReplaceAll(path, ":", "_")
	path = strings.ReplaceAll(path, "*", "star")
	path = strings.ReplaceAll(path, ".", "_")
	if path == "" {
		return "root"
	}
	return path
}

func endpointKey(ep Endpoint) string {
	return ep.Method + " " + ep.Path
}

var httpMethodOrder = []string{
	"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "CONNECT", "OPTIONS", "TRACE", "*",
}

func sortedHTTPMethods(methods map[string]string) []string {
	ordered := make([]string, 0, len(methods))
	seen := map[string]struct{}{}
	for _, method := range httpMethodOrder {
		if _, ok := methods[method]; ok {
			ordered = append(ordered, method)
			seen[method] = struct{}{}
		}
	}
	rest := make([]string, 0)
	for method := range methods {
		if _, ok := seen[method]; ok {
			continue
		}
		rest = append(rest, method)
	}
	sort.Strings(rest)
	return append(ordered, rest...)
}
