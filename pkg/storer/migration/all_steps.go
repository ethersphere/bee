// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/migration"
	"github.com/ethersphere/bee/pkg/storer/internal"
	"github.com/ethersphere/bee/pkg/storer/internal/reserve"
)

// AfterInitSteps lists all migration steps for localstore IndexStore after the localstore is intiated.
func AfterInitSteps(
	sharkyPath string,
	sharkyNoOfShards int,
	st internal.Storage,
) migration.Steps {
	return map[uint64]migration.StepFn{
		1: step_01,
		2: step_02(st),
		3: step_03(st, reserve.ChunkType),
		4: step_04(sharkyPath, sharkyNoOfShards, st),
	}
}

// BeforeInitSteps lists all migration steps for localstore IndexStore before the localstore is intiated.
func BeforeInitSteps(st storage.BatchedStore) migration.Steps {
	return map[uint64]migration.StepFn{
		1: RefCountSizeInc(st),
	}
}
