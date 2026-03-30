// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
)

// step_04 was a migration step that forced a sharky recovery on the localstore.
// It is now a NOOP since all nodes have already run this migration,
// and new nodes start with an empty database.
func step_04(_ string, _ int, _ transaction.Storage, _ log.Logger) func() error {
	return func() error {
		return nil
	}
}
