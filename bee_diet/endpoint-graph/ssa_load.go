// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

var skipPkgFragments = []string{"/testutil", "/testing", "/mock", "/test", "/storagetest"}

// utilityPackages are bee packages included as pkg/* nodes but not as Interface.Method capabilities.
var utilityPackages = map[string]struct{}{
	"pkg/log":            {},
	"pkg/jsonhttp":       {},
	"pkg/bigint":         {},
	"pkg/sctx":           {},
	"pkg/tracing":        {},
	"pkg/log/httpaccess": {},
}

func isUtilityPackage(pkg string) bool {
	if _, ok := utilityPackages[pkg]; ok {
		return true
	}
	return strings.HasPrefix(pkg, "pkg/log/")
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func loadSSA(beeRoot string) (*ssa.Program, error) {
	return loadSSAPatterns(beeRoot, beeModule+"/pkg/...")
}

func loadSSAProgram(beeRoot string) (*ssa.Program, error) {
	return loadSSAPatterns(beeRoot, beeModule+"/cmd/...", beeModule+"/pkg/...")
}

func loadSSAForEntry(beeRoot string, spec entrySpec) (*ssa.Program, error) {
	if strings.HasPrefix(spec.PkgRel, "pkg/") {
		return loadSSAPatterns(beeRoot, beeModule+"/pkg/...")
	}
	return loadSSAProgram(beeRoot)
}

func loadSSAPatterns(beeRoot string, patterns ...string) (*ssa.Program, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedImports |
			packages.NeedDeps |
			packages.NeedTypes |
			packages.NeedSyntax |
			packages.NeedTypesInfo,
		Dir:   beeRoot,
		Tests: false,
		Fset:  token.NewFileSet(),
	}

	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("packages.Load: %w", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		return nil, fmt.Errorf("package load errors")
	}

	prog, _ := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	prog.Build()
	return prog, nil
}

// entrySpec selects the program root function for -entry analysis.
type entrySpec struct {
	PkgPath  string
	PkgRel   string
	FuncName string
	Source   string
}

func parseEntrySpec(spec, beeRoot string) (entrySpec, error) {
	if spec == "" {
		return entrySpec{}, fmt.Errorf("empty entry spec")
	}
	s := strings.TrimSpace(spec)
	funcName := "main"
	source := ""
	pkgRel := "cmd/bee"

	if strings.Contains(s, ":") {
		parts := strings.SplitN(s, ":", 2)
		left, right := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if right != "" {
			funcName = right
		}
		if strings.HasSuffix(left, ".go") {
			source = left
			pkgRel = packageRelFromSource(beeRoot, left)
		} else {
			pkgRel = normalizeEntryPackage(left)
		}
	} else if strings.HasSuffix(s, ".go") {
		source = s
		pkgRel = packageRelFromSource(beeRoot, s)
	} else if s == "node" || s == "NewBee" {
		pkgRel = "pkg/node"
		funcName = "NewBee"
	} else if s == "main" {
		pkgRel = "cmd/bee"
	} else if strings.Contains(s, "/") {
		pkgRel = normalizeEntryPackage(s)
	} else {
		funcName = s
	}

	return entrySpec{
		PkgPath:  beeModule + "/" + pkgRel,
		PkgRel:   pkgRel,
		FuncName: funcName,
		Source:   source,
	}, nil
}

func normalizeEntryPackage(pkg string) string {
	pkg = strings.TrimPrefix(strings.TrimSpace(pkg), beeModule+"/")
	pkg = strings.TrimPrefix(pkg, "./")
	return strings.TrimSuffix(pkg, "/")
}

func packageRelFromSource(beeRoot, source string) string {
	source = strings.TrimPrefix(source, beeRoot)
	source = strings.TrimPrefix(source, "/")
	source = strings.TrimPrefix(source, "./")
	dir := filepath.Dir(source)
	return normalizeEntryPackage(dir)
}

func findEntryFunctions(prog *ssa.Program, spec entrySpec) []*ssa.Function {
	var matches []*ssa.Function
	for fn := range ssautil.AllFunctions(prog) {
		if fn.Name() != spec.FuncName {
			continue
		}
		if fn.Pkg == nil || fn.Pkg.Pkg == nil {
			continue
		}
		if fn.Pkg.Pkg.Path() != spec.PkgPath {
			continue
		}
		if fn.Signature.Recv() != nil {
			continue
		}
		matches = append(matches, fn)
	}
	return matches
}

func beeProjectPath(fn *ssa.Function) (string, bool) {
	if fn.Pkg == nil || fn.Pkg.Pkg == nil {
		return "", false
	}
	path := fn.Pkg.Pkg.Path()
	if !strings.HasPrefix(path, beeModule+"/") {
		return "", false
	}
	rel := strings.TrimPrefix(path, beeModule+"/")
	if !strings.HasPrefix(rel, "pkg/") && !strings.HasPrefix(rel, "cmd/") {
		return "", false
	}
	for _, frag := range skipPkgFragments {
		if strings.Contains(rel, frag) {
			return "", false
		}
	}
	return rel, true
}

func beePkgPath(fn *ssa.Function) (string, bool) {
	rel, ok := beeProjectPath(fn)
	if !ok || !strings.HasPrefix(rel, "pkg/") {
		return "", false
	}
	return rel, true
}

func fnCallRef(fn *ssa.Function) string {
	if fn == nil {
		return "unknown"
	}
	rel, ok := beeProjectPath(fn)
	if !ok {
		return fn.String()
	}
	if recv := fn.Signature.Recv(); recv != nil {
		return rel + ".(" + shortBeeType(recv.Type().String()) + ")." + fn.Name()
	}
	return rel + "." + fn.Name()
}

func constructorRef(fn *ssa.Function) string {
	rel, ok := beeProjectPath(fn)
	if !ok {
		return ""
	}
	return rel + "." + fn.Name()
}

func findHandlerFuncs(prog *ssa.Program, handler string) []*ssa.Function {
	if handler == "" || handler == "unknown" || handler == "anonymous" || handler == "external" || isExternalHandler(handler) {
		return nil
	}
	if strings.HasPrefix(handler, "stdlib:") {
		return nil
	}

	apiPkg := beeModule + "/pkg/api"
	name := handler
	if strings.Contains(handler, ".") {
		parts := strings.Split(handler, ".")
		name = parts[len(parts)-1]
	}

	var matches []*ssa.Function
	for fn := range ssautil.AllFunctions(prog) {
		if fn.Name() != name {
			continue
		}
		if fn.Pkg == nil || fn.Pkg.Pkg == nil {
			continue
		}
		pkgPath := fn.Pkg.Pkg.Path()
		if pkgPath != apiPkg && !strings.HasSuffix(handler, fn.Name()) {
			continue
		}
		if pkgPath == apiPkg || strings.HasPrefix(handler, "pprof.") {
			matches = append(matches, fn)
		}
	}
	return matches
}

func findMiddlewareClosures(fn *ssa.Function) []*ssa.Function {
	if fn == nil {
		return nil
	}
	var out []*ssa.Function
	for _, b := range fn.Blocks {
		for _, ins := range b.Instrs {
			mc, ok := ins.(*ssa.MakeClosure)
			if !ok {
				continue
			}
			cl, ok := mc.Fn.(*ssa.Function)
			if !ok {
				continue
			}
			out = append(out, cl)
		}
	}
	return out
}

func fnLabel(fn *ssa.Function) string {
	if fn.Pkg == nil || fn.Pkg.Pkg == nil {
		return fn.String()
	}
	recv := fn.Signature.Recv()
	if recv != nil {
		return fmt.Sprintf("(%s).%s", recv.Type().String(), fn.Name())
	}
	return fn.Pkg.Pkg.Name() + "." + fn.Name()
}
