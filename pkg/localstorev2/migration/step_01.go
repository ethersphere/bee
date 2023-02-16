// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import (
	storage "github.com/ethersphere/bee/pkg/storagev2"
)

// step_01 serves as example for setting up migration step.
//
// In this step store is not being modified.
func step_01(s storage.Store) error {
	// NOOP
	return nil
}
