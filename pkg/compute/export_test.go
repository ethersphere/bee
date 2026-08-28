// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package compute

import (
	"context"

	"github.com/tetratelabs/wazero"
)

// SwarmExports is the import allowlist checkImports enforces.
var SwarmExports = swarmExports

// SwarmModuleExports instantiates the swarm host module and reports the names
// it actually defines, so a test can hold it against SwarmExports.
func SwarmModuleExports(ctx context.Context) ([]string, error) {
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	if err := buildSwarmModule(ctx, r, &hostState{}); err != nil {
		return nil, err
	}

	var names []string
	for name := range r.Module(swarmModuleName).ExportedFunctionDefinitions() {
		names = append(names, name)
	}
	return names, nil
}
