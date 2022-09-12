// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldbstore_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/storagev2/leveldbstore"
	ldb "github.com/ethersphere/bee/pkg/storagev2/leveldbstore"
	"github.com/ethersphere/bee/pkg/storagev2/storagetest"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func TestStoreTestSuite(t *testing.T) {
	store, err := leveldbstore.New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	storagetest.TestStore(t, store)
}

func BenchmarkStore(b *testing.B) {
	st, err := ldb.New("", &opt.Options{
		Compression: opt.SnappyCompression,
	})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = st.Close() })
	storagetest.RunStoreBenchmarkTests(b, st)
}
