// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package snapshot

import (
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethersphere/bee/v2/pkg/log"
)

var BlockPage = blockPage

// NewFilterer builds a filterer over logs sorted by block number, bypassing
// Parse, for tests of FilterLogs alone.
func NewFilterer(logs []types.Log) *SnapshotLogFilterer {
	f := &SnapshotLogFilterer{logger: log.Noop, logs: logs}
	if len(logs) > 0 {
		f.maxBlock = logs[len(logs)-1].BlockNumber
	}
	return f
}
