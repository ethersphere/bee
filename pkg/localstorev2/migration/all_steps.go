// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import "github.com/ethersphere/bee/pkg/storagev2/migration"

// AllSteps lists all migration steps for localstore IndexStore.
func AllSteps() migration.Steps {
	return map[uint64]migration.StepFn{
		// 2: step_02,
		1: step_01,
	}
}
