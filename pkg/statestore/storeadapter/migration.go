// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storeadapter

import (
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/migration"
)

// allSteps lists all state store migration steps.
// All steps are now NOOPs since all nodes have already run these migrations,
// and new nodes start with an empty database.
func allSteps(_ storage.Store) migration.Steps {
	noop := func() error { return nil }
	return map[uint64]migration.StepFn{
		1: noop,
		2: noop,
		3: noop,
		4: noop,
		5: noop,
		6: noop,
		7: noop,
		8: noop,
	}
}
