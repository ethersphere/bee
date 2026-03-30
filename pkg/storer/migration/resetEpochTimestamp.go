// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import "github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"

// resetReserveEpochTimestamp was a migration that reset the epoch timestamp
// of the reserve so that peers in the network could resync chunks.
// It is now a NOOP since all nodes have already run this migration,
// and new nodes start with an empty database.
func resetReserveEpochTimestamp(_ transaction.Storage) func() error {
	return func() error {
		return nil
	}
}
