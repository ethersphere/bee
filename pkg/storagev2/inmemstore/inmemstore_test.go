// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmem_test

import (
	"testing"

	inmem "github.com/ethersphere/bee/pkg/storagev2/inmemstore"
	storetesting "github.com/ethersphere/bee/pkg/storagev2/testsuite"
)

func TestStoreTestSuite(t *testing.T) {
	st := inmem.New()
	storetesting.RunCorrectnessTests(t, st)
}
