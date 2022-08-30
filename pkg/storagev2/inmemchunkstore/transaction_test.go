// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package inmemchunkstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/storagev2"
	inmem "github.com/ethersphere/bee/pkg/storagev2/inmemchunkstore"
	"github.com/ethersphere/bee/pkg/storagev2/storagetest"
)

func TestTxChunkStore(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	chunkStore := new(inmem.TxChunkStore).NewTx(&storage.TxChunkStoreBase{
		TxState:    storage.NewTxState(ctx),
		ChunkStore: inmem.New(),
	})

	// We need to call Commit() so the chunkStore.Close() method won't block.
	time.AfterFunc(100*time.Millisecond, func() {
		if err := chunkStore.Commit(); err != nil {
			t.Fatalf("Commit(): unexpected error: %v", err)
		}
	})

	storagetest.TestChunkStore(t, chunkStore)
}

// TODO: test the Rollback functionality.
