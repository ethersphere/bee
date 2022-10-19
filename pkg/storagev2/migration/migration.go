// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration

import storage "github.com/ethersphere/bee/pkg/storagev2"

// To be implemented in #3401
type StepFn func(storage.Store) error
