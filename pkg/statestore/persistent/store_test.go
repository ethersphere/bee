// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.package storage

package persistent_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/statestore/mock"
	"github.com/ethersphere/bee/pkg/statestore/test"
	"github.com/ethersphere/bee/pkg/storage"
)

func TestPersistentStateStore(t *testing.T) {
	test.Run(t, func(t *testing.T) (storage.StateStorer, func()) {
		return mock.NewStateStore(), func() {}
	})
}
