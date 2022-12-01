// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemstore_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/storagev2/inmemstore"
	"github.com/ethersphere/bee/pkg/storagev2/storagetest"
)

func TestTxStore(t *testing.T) {
	t.Parallel()

	storagetest.TestTxStore(t, inmemstore.NewTxStore(inmemstore.New()))
}
