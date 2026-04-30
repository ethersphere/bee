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
	// IMPORTANT: keep this noop and all historical version entries that point to it.
	// migration.Migrate executes steps starting from (storedVersion + 1) and stops
	// at the first missing version. If an old NOOP version is removed/renumbered,
	// nodes that already stored a higher version can start beyond the new steps
	// and never execute newly added migrations.
	return map[uint64]migration.StepFn{
		1: legacyNoopStep,
		2: legacyNoopStep,
		3: legacyNoopStep,
		4: legacyNoopStep,
		5: legacyNoopStep,
		6: legacyNoopStep,
		7: legacyNoopStep,
	}
}

// BeforeInitSteps lists all migration steps for localstore IndexStore before the localstore is initiated.
func BeforeInitSteps(st storage.BatchStore, logger log.Logger) migration.Steps {
	return map[uint64]migration.StepFn{
		1: legacyNoopStep,
	}
}

func legacyNoopStep() error {
	return nil
}
