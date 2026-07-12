// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

type invokeIndex struct {
	byName map[string][]*ssa.Function
	wiring *wiringIndex
	fields map[string]serviceField
}

type invokeResolution struct {
	impls     []*ssa.Function
	ambiguous bool
	implCount int
}

func newInvokeIndex(prog *ssa.Program, fields map[string]serviceField) *invokeIndex {
	idx := &invokeIndex{
		byName: make(map[string][]*ssa.Function),
		wiring: newWiringIndex(),
		fields: fields,
	}
	for fn := range ssautil.AllFunctions(prog) {
		if fn.Signature.Recv() == nil {
			continue
		}
		if types.IsInterface(fn.Signature.Recv().Type().Underlying()) {
			continue
		}
		if _, ok := beePkgPath(fn); !ok {
			continue
		}
		idx.byName[fn.Name()] = append(idx.byName[fn.Name()], fn)
	}
	return idx
}

func (idx *invokeIndex) implementations(call ssa.CallInstruction, ifaceType types.Type) invokeResolution {
	if !call.Common().IsInvoke() {
		return invokeResolution{}
	}
	method := call.Common().Method
	if method == nil || ifaceType == nil {
		return invokeResolution{}
	}
	iface, ok := ifaceType.Underlying().(*types.Interface)
	if !ok || !shouldExpandInvoke(method) {
		return invokeResolution{}
	}

	ifaceKey := types.TypeString(ifaceType, nil)
	if implPkg := idx.wiring.implPackage(ifaceKey); implPkg != "" {
		var out []*ssa.Function
		for _, fn := range idx.byName[method.Name()] {
			if !methodMatches(method, fn) {
				continue
			}
			pkg, ok := beePkgPath(fn)
			if !ok || pkg != implPkg {
				continue
			}
			recvType := fn.Signature.Recv().Type()
			ptr := recvType
			if _, ok := recvType.(*types.Pointer); !ok {
				ptr = types.NewPointer(recvType)
			}
			if !types.Implements(ptr, iface) {
				continue
			}
			out = append(out, fn)
		}
		if len(out) > 0 {
			return invokeResolution{impls: out, implCount: len(out)}
		}
	}

	var out []*ssa.Function
	for _, fn := range idx.byName[method.Name()] {
		if !methodMatches(method, fn) {
			continue
		}
		recvType := fn.Signature.Recv().Type()
		ptr := recvType
		if _, ok := recvType.(*types.Pointer); !ok {
			ptr = types.NewPointer(recvType)
		}
		if !types.Implements(ptr, iface) {
			continue
		}
		pkg, ok := beePkgPath(fn)
		if !ok || strings.Contains(pkg, "/mock") {
			continue
		}
		out = append(out, fn)
	}
	if len(out) > 8 {
		return invokeResolution{ambiguous: true, implCount: len(out)}
	}
	return invokeResolution{impls: out, implCount: len(out)}
}

func shouldExpandInvoke(method *types.Func) bool {
	if method == nil {
		return false
	}
	name := method.Name()
	if strings.HasPrefix(name, "XXX_") {
		return false
	}
	pkg := method.Pkg()
	if pkg == nil || !strings.HasPrefix(pkg.Path(), beeModule+"/pkg/") {
		return false
	}
	for _, frag := range skipPkgFragments {
		if strings.Contains(pkg.Path(), frag) {
			return false
		}
	}
	switch name {
	case "Error", "String", "GoString", "Format",
		"Marshal", "Unmarshal", "MarshalJSON", "UnmarshalJSON",
		"Lock", "Unlock", "RLock", "RUnlock",
		"Len", "Cap", "Close", "Read", "Write":
		return false
	}
	return true
}

func methodMatches(ifaceMethod *types.Func, impl *ssa.Function) bool {
	ifaceSig, ok := ifaceMethod.Type().(*types.Signature)
	if !ok {
		return false
	}
	implSig := impl.Signature
	return types.Identical(ifaceSig.Params(), implSig.Params()) &&
		types.Identical(ifaceSig.Results(), implSig.Results())
}

func serviceFieldFromValue(val ssa.Value, idx *invokeIndex) string {
	for depth := 0; depth < 8 && val != nil; depth++ {
		switch v := val.(type) {
		case *ssa.FieldAddr:
			if _, ok := v.X.(*ssa.FreeVar); ok {
				name := fieldNameFromFieldAddr(v)
				if name == "" || !isCapabilityField(name) {
					break
				}
				return name
			}
		case *ssa.UnOp:
			if v.Op == token.MUL || v.Op == token.AND {
				val = v.X
				continue
			}
			if v.Op == token.ARROW {
				val = v.X
				continue
			}
		}
		break
	}
	return ""
}

func fieldNameFromFieldAddr(v *ssa.FieldAddr) string {
	t := v.X.Type()
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	st, ok := t.Underlying().(*types.Struct)
	if !ok || v.Field < 0 || v.Field >= st.NumFields() {
		return ""
	}
	return st.Field(v.Field).Name()
}
