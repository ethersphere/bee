package migration

import storage "github.com/ethersphere/bee/pkg/storagev2"

// To be implemented in #3401
type StepFn func(storage.Store) error
