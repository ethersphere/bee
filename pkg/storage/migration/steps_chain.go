// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import storage "github.com/ethersphere/bee/pkg/storage"

// NewStepsChain returns new StepFn which combines all supplied StepFn
// into single StepFn.
func NewStepsChain(steps ...StepFn) StepFn {
	return func(s storage.Store) error {
		for _, stepFn := range steps {
			if err := stepFn(s); err != nil {
				return err
			}
		}

		return nil
	}
}
