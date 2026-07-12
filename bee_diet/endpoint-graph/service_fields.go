// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

// serviceField maps an api.Service field name to its interface/type string.
type serviceField struct {
	Field     string
	Type      string
	Interface string
	Package   string
}

var defaultServiceFields = []serviceField{
	{Field: "storer", Type: "Storer", Interface: beeModule + "/pkg/api.Storer", Package: "pkg/api"},
	{Field: "resolver", Interface: beeModule + "/pkg/resolver.Interface", Package: "pkg/resolver"},
	{Field: "pss", Interface: beeModule + "/pkg/pss.Interface", Package: "pkg/pss"},
	{Field: "gsoc", Interface: beeModule + "/pkg/gsoc.Listener", Package: "pkg/gsoc"},
	{Field: "steward", Interface: beeModule + "/pkg/steward.Interface", Package: "pkg/steward"},
	{Field: "feedFactory", Interface: beeModule + "/pkg/feeds.Factory", Package: "pkg/feeds"},
	{Field: "post", Interface: beeModule + "/pkg/postage.Service", Package: "pkg/postage"},
	{Field: "accesscontrol", Interface: beeModule + "/pkg/accesscontrol.Controller", Package: "pkg/accesscontrol"},
	{Field: "postageContract", Interface: beeModule + "/pkg/postage/postagecontract.Interface", Package: "pkg/postage/postagecontract"},
	{Field: "stakingContract", Interface: beeModule + "/pkg/storageincentives/staking.Contract", Package: "pkg/storageincentives/staking"},
	{Field: "topologyDriver", Interface: beeModule + "/pkg/topology.Driver", Package: "pkg/topology"},
	{Field: "p2p", Interface: beeModule + "/pkg/p2p.DebugService", Package: "pkg/p2p"},
	{Field: "accounting", Interface: beeModule + "/pkg/accounting.Interface", Package: "pkg/accounting"},
	{Field: "chequebook", Interface: beeModule + "/pkg/settlement/swap/chequebook.Service", Package: "pkg/settlement/swap/chequebook"},
	{Field: "pseudosettle", Interface: beeModule + "/pkg/settlement.Interface", Package: "pkg/settlement"},
	{Field: "pingpong", Interface: beeModule + "/pkg/pingpong.Interface", Package: "pkg/pingpong"},
	{Field: "batchStore", Interface: beeModule + "/pkg/postage.Storer", Package: "pkg/postage"},
	{Field: "stamperStore", Interface: beeModule + "/pkg/storage.Store", Package: "pkg/storage"},
	{Field: "swap", Interface: beeModule + "/pkg/settlement/swap.Interface", Package: "pkg/settlement/swap"},
	{Field: "transaction", Interface: beeModule + "/pkg/transaction.Service", Package: "pkg/transaction"},
	{Field: "lightNodes", Type: "*lightnode.Container", Package: "pkg/lightnode"},
	{Field: "chainBackend", Interface: beeModule + "/pkg/transaction.Backend", Package: "pkg/transaction"},
	{Field: "erc20Service", Interface: beeModule + "/pkg/settlement/swap/erc20.Service", Package: "pkg/settlement/swap/erc20"},
	{Field: "redistributionAgent", Type: "*storageincentives.Agent", Package: "pkg/storageincentives"},
	{Field: "statusService", Type: "*status.Service", Package: "pkg/status"},
	{Field: "pinIntegrity", Interface: beeModule + "/pkg/api.PinIntegrity", Package: "pkg/api"},
	{Field: "envelopeStorage", Interface: beeModule + "/contracts/client.Client", Package: "contracts/client"},
}

var nonCapabilityServiceFields = map[string]struct{}{
	"logger": {}, "loggerV1": {}, "tracer": {}, "signer": {}, "probe": {},
	"metricsRegistry": {}, "router": {}, "metrics": {}, "wsWg": {}, "quit": {},
	"overlay": {}, "publicKey": {}, "pssPublicKey": {}, "ethereumAddress": {},
	"chequebookEnabled": {}, "swapEnabled": {}, "fullAPIEnabled": {}, "syncStatus": {},
	"blockTime": {}, "statusSem": {}, "postageSem": {}, "stakingSem": {},
	"cashOutChequeSem": {}, "beeMode": {}, "chainID": {},
	"whitelistedWithdrawalAddress": {}, "preMapHooks": {}, "customValidationMessages": {},
	"validate": {}, "isWarmingUp": {}, "Options": {}, "Handler": {},
}

// loadServiceFields resolves api.Service fields via go/types and merges defaults.
func loadServiceFields(beeRoot string) (map[string]serviceField, error) {
	fields := make(map[string]serviceField, len(defaultServiceFields))
	for _, f := range defaultServiceFields {
		fields[f.Field] = f
	}
	syncCapabilityFieldNames(fields)

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedSyntax,
		Dir:  beeRoot,
		Fset: token.NewFileSet(),
	}
	pkgs, err := packages.Load(cfg, beeModule+"/pkg/api")
	if err != nil {
		return fields, fmt.Errorf("packages.Load: %w", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		return fields, fmt.Errorf("package load errors")
	}
	if len(pkgs) == 0 || pkgs[0].Types == nil {
		return fields, nil
	}

	obj := pkgs[0].Types.Scope().Lookup("Service")
	named, ok := obj.(*types.TypeName)
	if !ok {
		return fields, nil
	}
	st, ok := named.Type().Underlying().(*types.Struct)
	if !ok {
		return fields, nil
	}

	for i := 0; i < st.NumFields(); i++ {
		field := st.Field(i)
		if !field.Exported() {
			continue
		}
		name := field.Name()
		if isNonCapabilityServiceField(name) {
			continue
		}
		sf := serviceFieldFromTypes(name, field.Type())
		if sf.Interface == "" && sf.Package == "" {
			continue
		}
		if prev, ok := fields[name]; ok {
			if sf.Interface == "" {
				sf.Interface = prev.Interface
			}
			if sf.Package == "" {
				sf.Package = prev.Package
			}
			if sf.Type == "" {
				sf.Type = prev.Type
			}
		}
		fields[name] = sf
	}

	syncCapabilityFieldNames(fields)
	return fields, nil
}

func isNonCapabilityServiceField(name string) bool {
	if _, ok := nonCapabilityServiceFields[name]; ok {
		return true
	}
	return strings.HasSuffix(name, "Sem")
}

func serviceFieldFromTypes(name string, typ types.Type) serviceField {
	sf := serviceField{Field: name}
	typ = types.Unalias(typ)
	if ptr, ok := typ.(*types.Pointer); ok {
		typ = ptr.Elem()
		sf.Type = "*" + typeNameString(typ)
	} else {
		sf.Type = typeNameString(typ)
	}

	switch t := typ.(type) {
	case *types.Named:
		if obj := t.Obj(); obj != nil && obj.Pkg() != nil {
			pkgPath := obj.Pkg().Path()
			if strings.HasPrefix(pkgPath, beeModule+"/") {
				sf.Interface = pkgPath + "." + obj.Name()
				sf.Package = shortBeePkg(pkgPath)
			}
		}
	case *types.Interface:
		sf.Interface = types.TypeString(typ, nil)
		if pkg := packageFromInterface(sf.Interface); pkg != "" {
			sf.Package = pkg
		}
	}
	return sf
}

func typeNameString(typ types.Type) string {
	typ = types.Unalias(typ)
	if named, ok := typ.(*types.Named); ok {
		if obj := named.Obj(); obj != nil {
			return obj.Name()
		}
	}
	return typ.String()
}

func syncCapabilityFieldNames(fields map[string]serviceField) {
	capabilityFieldNames = make(map[string]struct{}, len(fields))
	for name := range fields {
		capabilityFieldNames[name] = struct{}{}
	}
}

func (sf serviceField) nodeID() string {
	return "field:" + sf.Field
}
