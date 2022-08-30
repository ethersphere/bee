// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/storagev2"
	inmem "github.com/ethersphere/bee/pkg/storagev2/inmemstore"
	"github.com/ethersphere/bee/pkg/storagev2/storagetest"
)

func TestTxStore(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	store := inmem.NewTxStore(&storage.TxStoreBase{
		TxState: storage.NewTxState(ctx),
		Store:   inmem.New(),
	})

	// We need to call Commit() so the store.Close() method won't block.
	time.AfterFunc(100*time.Millisecond, func() {
		if err := store.Commit(); err != nil {
			t.Fatalf("Commit(): unexpected error: %v", err)
		}
	})

	storagetest.TestStore(t, store)
}

// TODO: test the Rollback functionality.