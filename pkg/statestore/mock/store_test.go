// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mock_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/statestore/mock"
	"github.com/ethersphere/bee/v2/pkg/statestore/test"
	"github.com/ethersphere/bee/v2/pkg/storage"
)

func TestMockStateStore(t *testing.T) {
	t.Parallel()
	test.Run(t, func(t *testing.T) storage.StateStorer {
		t.Helper()
		return mock.NewStateStore()
	})
}
