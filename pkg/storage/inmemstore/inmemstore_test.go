// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemstore_test

import (
	"testing"

	inmem "github.com/ethersphere/bee/v2/pkg/storage/inmemstore"
	"github.com/ethersphere/bee/v2/pkg/storage/storagetest"
)

func TestStore(t *testing.T) {
	t.Parallel()

	storagetest.TestStore(t, inmem.New())
}

func BenchmarkStore(b *testing.B) {
	storagetest.BenchmarkStore(b, inmem.New())
}

func TestBatchedStore(t *testing.T) {
	t.Parallel()

	storagetest.TestBatchedStore(t, inmem.New())
}

func BenchmarkBatchedStore(b *testing.B) {
	storagetest.BenchmarkBatchedStore(b, inmem.New())
}
