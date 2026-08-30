// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compute

import (
	"context"

	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/tetratelabs/wazero"
)

// SwarmResponseExports is the half of the import allowlist that needs no Host.
var SwarmResponseExports = swarmResponseExports

// SwarmHostExports is the half of the import allowlist that reaches the node.
var SwarmHostExports = swarmHostExports

// SwarmExports is the whole allowlist checkImports enforces when a Host is
// available.
func SwarmExports() map[string]struct{} {
	all := make(map[string]struct{}, len(swarmResponseExports)+len(swarmHostExports))
	for name := range swarmResponseExports {
		all[name] = struct{}{}
	}
	for name := range swarmHostExports {
		all[name] = struct{}{}
	}
	return all
}

// SwarmModuleExports instantiates the swarm host module and reports the names it
// actually defines, so a test can hold it against the allowlists. hostAvailable
// selects whether the data functions are registered, mirroring a node running
// with and without node access.
func SwarmModuleExports(ctx context.Context, hostAvailable bool) ([]string, error) {
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	hs := &hostState{}
	if hostAvailable {
		hs.host = noopHost{}
	}
	if err := buildSwarmModule(ctx, r, hs); err != nil {
		return nil, err
	}

	var names []string
	for name := range r.Module(swarmModuleName).ExportedFunctionDefinitions() {
		names = append(names, name)
	}
	return names, nil
}

// noopHost stands in for a Host so buildSwarmModule registers the data half. It
// is never called: the test only inspects the module's export list.
type noopHost struct{}

func (noopHost) BytesGet(context.Context, swarm.Address) ([]byte, error) {
	return nil, ErrNotFound
}
func (noopHost) BytesPut(context.Context, []byte, []byte) (swarm.Address, error) {
	return swarm.ZeroAddress, ErrDenied
}
func (noopHost) ChunkGet(context.Context, swarm.Address) ([]byte, error) {
	return nil, ErrNotFound
}
func (noopHost) ChunkPut(context.Context, []byte, []byte) (swarm.Address, error) {
	return swarm.ZeroAddress, ErrDenied
}
