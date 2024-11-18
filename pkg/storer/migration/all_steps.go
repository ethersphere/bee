// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/migration"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
)

// AfterInitSteps lists all migration steps for localstore IndexStore after the localstore is initiated.
func AfterInitSteps(
	sharkyPath string,
	sharkyNoOfShards int,
	st transaction.Storage,
	logger log.Logger,
) migration.Steps {
	return map[uint64]migration.StepFn{
		1: step_01,
		2: step_02(st),
		3: ReserveRepairer(st, storage.ChunkType, logger),
		4: step_04(sharkyPath, sharkyNoOfShards, st, logger),
		5: step_05(st, logger),
		6: step_06(st, logger),
		7: resetReserveEpochTimestamp(st),
	}
}

// BeforeInitSteps lists all migration steps for localstore IndexStore before the localstore is initiated.
func BeforeInitSteps(st storage.BatchStore, logger log.Logger) migration.Steps {
	return map[uint64]migration.StepFn{
		1: RefCountSizeInc(st, logger),
	}
}
