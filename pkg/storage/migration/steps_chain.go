// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

// NewStepsChain returns new StepFn which combines all supplied StepFn
// into single StepFn.
func NewStepsChain(steps ...StepFn) StepFn {
	return func() error {
		for _, stepFn := range steps {
			if err := stepFn(); err != nil {
				return err
			}
		}

		return nil
	}
}
