// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import "github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"

// step_02 was a migration step that migrated the cache to a new format.
// It is now a NOOP since all nodes have already run this migration,
// and new nodes start with an empty database.
func step_02(_ transaction.Storage) func() error {
	return func() error {
		return nil
	}
}
