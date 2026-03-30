// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"
)

// RefCountSizeInc was a migration step that replaced chunkstore items
// to increase refCnt capacity from uint8 to uint32.
// It is now a NOOP since all nodes have already run this migration,
// and new nodes start with an empty database.
func RefCountSizeInc(_ storage.BatchStore, _ log.Logger) func() error {
	return func() error {
		return nil
	}
}
