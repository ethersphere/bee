// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemstore_test

import (
	"testing"

	inmem "github.com/ethersphere/bee/pkg/storagev2/inmemstore"
	"github.com/ethersphere/bee/pkg/storagev2/storagetest"
)

func TestStore(t *testing.T) {
	storagetest.TestStore(t, inmem.New())
}

func BenchmarkStore(b *testing.B) {
	st := inmem.New()
	storagetest.RunStoreBenchmarkTests(b, st)
}
