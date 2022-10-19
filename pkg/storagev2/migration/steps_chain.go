package migration

import storage "github.com/ethersphere/bee/pkg/storagev2"

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
