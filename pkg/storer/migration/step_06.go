// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
)

// step_06 was a migration step that added a stampHash to all
// BatchRadiusItems, ChunkBinItems and StampIndexItems.
// It is now a NOOP since all nodes have already run this migration,
// and new nodes start with an empty database.
func step_06(_ transaction.Storage, _ log.Logger) func() error {
	return func() error {
		return nil
	}
}
