// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	pullerMock "github.com/ethersphere/bee/v2/pkg/puller/mock"
	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage"
	chunk "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// TestRecoveryPrunesCorruptedChunks verifies that on restart after an unclean
// shutdown, validateAndAddLocations removes index entries whose Sharky data no
// longer validates (hash mismatch), while leaving intact entries unaffected.
func TestRecoveryPrunesCorruptedChunks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	basePath := t.TempDir()
	opts := dbTestOps(swarm.RandAddress(t), 1000, nil, nil, time.Minute)

	batch := postagetesting.MustNewBatch()

	// 1. Open the storer and write two chunks.
	st, err := storer.New(ctx, basePath, opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	readyC := make(chan struct{})
	st.StartReserveWorker(ctx, pullerMock.NewMockRateReporter(0), networkRadiusFunc(0), readyC)
	<-readyC

	goodChunk := chunk.GenerateTestRandomChunk().WithStamp(postagetesting.MustNewBatchStamp(batch.ID))
	badChunk := chunk.GenerateTestRandomChunk().WithStamp(postagetesting.MustNewBatchStamp(batch.ID))

	putter := st.ReservePutter()
	if err := putter.Put(ctx, goodChunk); err != nil {
		t.Fatalf("Put good chunk: %v", err)
	}
	if err := putter.Put(ctx, badChunk); err != nil {
		t.Fatalf("Put bad chunk: %v", err)
	}

	// 2. Locate the bad chunk's Sharky slot before closing.
	var badLoc sharky.Location
	if err := st.Storage().IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return new(chunkstore.RetrievalIndexItem) },
	}, func(r storage.Result) (bool, error) {
		item := r.Entry.(*chunkstore.RetrievalIndexItem)
		if item.Address.Equal(badChunk.Address()) {
			badLoc = item.Location
		}
		return false, nil
	}); err != nil {
		t.Fatalf("Iterate index: %v", err)
	}
	if badLoc.Length == 0 {
		t.Fatal("bad chunk not found in index")
	}

	// 3. Close cleanly (removes .DIRTY).
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// 4. Simulate unclean shutdown: recreate .DIRTY so recovery runs on next open.
	dirtyPath := filepath.Join(basePath, "sharky", ".DIRTY")
	if err := os.WriteFile(dirtyPath, []byte{}, 0o644); err != nil {
		t.Fatalf("WriteFile .DIRTY: %v", err)
	}

	// 5. Overwrite the bad chunk's slot with zeros so its hash will not validate.
	shardPath := filepath.Join(basePath, "sharky", fmt.Sprintf("shard_%03d", badLoc.Shard))
	f, err := os.OpenFile(shardPath, os.O_RDWR, 0o666)
	if err != nil {
		t.Fatalf("OpenFile shard: %v", err)
	}
	if _, err := f.WriteAt(make([]byte, badLoc.Length), int64(badLoc.Slot)*int64(swarm.SocMaxChunkSize)); err != nil {
		_ = f.Close()
		t.Fatalf("WriteAt zeros: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close shard: %v", err)
	}

	// 6. Reopen the storer. The .DIRTY file triggers sharkyRecovery →
	// validateAndAddLocations, which prunes the corrupted index entry.
	st2, err := storer.New(ctx, basePath, opts)
	if err != nil {
		t.Fatalf("New (reopen): %v", err)
	}
	t.Cleanup(func() { _ = st2.Close() })

	// 7. The valid chunk must still be retrievable after recovery.
	got, err := st2.Storage().ChunkStore().Get(ctx, goodChunk.Address())
	if err != nil {
		t.Fatalf("Get good chunk after recovery: %v", err)
	}
	if !got.Address().Equal(goodChunk.Address()) {
		t.Fatalf("good chunk address mismatch after recovery")
	}

	// 8. The corrupted chunk must have been pruned from the index.
	_, err = st2.Storage().ChunkStore().Get(ctx, badChunk.Address())
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for corrupted chunk, got: %v", err)
	}
}

// TestRecoveryPrunesUnreadableChunks verifies that on restart after an unclean
// shutdown, validateAndAddLocations removes index entries whose Sharky slot
// cannot be read at all (Read error path), while leaving intact entries unaffected.
func TestRecoveryPrunesUnreadableChunks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	basePath := t.TempDir()
	opts := dbTestOps(swarm.RandAddress(t), 1000, nil, nil, time.Minute)

	batch := postagetesting.MustNewBatch()

	// 1. Open the storer and write two chunks.
	st, err := storer.New(ctx, basePath, opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	readyC := make(chan struct{})
	st.StartReserveWorker(ctx, pullerMock.NewMockRateReporter(0), networkRadiusFunc(0), readyC)
	<-readyC

	goodChunk := chunk.GenerateTestRandomChunk().WithStamp(postagetesting.MustNewBatchStamp(batch.ID))
	badChunk := chunk.GenerateTestRandomChunk().WithStamp(postagetesting.MustNewBatchStamp(batch.ID))

	putter := st.ReservePutter()
	if err := putter.Put(ctx, goodChunk); err != nil {
		t.Fatalf("Put good chunk: %v", err)
	}
	if err := putter.Put(ctx, badChunk); err != nil {
		t.Fatalf("Put bad chunk: %v", err)
	}

	// 2. Locate the bad chunk's Sharky slot before closing.
	var badLoc sharky.Location
	if err := st.Storage().IndexStore().Iterate(storage.Query{
		Factory: func() storage.Item { return new(chunkstore.RetrievalIndexItem) },
	}, func(r storage.Result) (bool, error) {
		item := r.Entry.(*chunkstore.RetrievalIndexItem)
		if item.Address.Equal(badChunk.Address()) {
			badLoc = item.Location
		}
		return false, nil
	}); err != nil {
		t.Fatalf("Iterate index: %v", err)
	}
	if badLoc.Length == 0 {
		t.Fatal("bad chunk not found in index")
	}

	// 3. Close cleanly (removes .DIRTY).
	if err := st.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// 4. Simulate unclean shutdown: recreate .DIRTY so recovery runs on next open.
	dirtyPath := filepath.Join(basePath, "sharky", ".DIRTY")
	if err := os.WriteFile(dirtyPath, []byte{}, 0o644); err != nil {
		t.Fatalf("WriteFile .DIRTY: %v", err)
	}

	// 5. Truncate the shard file to the start of the bad chunk's slot so that
	// ReadAt returns EOF — simulating data that was never flushed to disk.
	shardPath := filepath.Join(basePath, "sharky", fmt.Sprintf("shard_%03d", badLoc.Shard))
	if err := os.Truncate(shardPath, int64(badLoc.Slot)*int64(swarm.SocMaxChunkSize)); err != nil {
		t.Fatalf("Truncate shard: %v", err)
	}

	// 6. Reopen the storer. Recovery must detect the unreadable slot and prune it.
	st2, err := storer.New(ctx, basePath, opts)
	if err != nil {
		t.Fatalf("New (reopen): %v", err)
	}
	t.Cleanup(func() { _ = st2.Close() })

	// 7. The valid chunk must still be retrievable after recovery.
	got, err := st2.Storage().ChunkStore().Get(ctx, goodChunk.Address())
	if err != nil {
		t.Fatalf("Get good chunk after recovery: %v", err)
	}
	if !got.Address().Equal(goodChunk.Address()) {
		t.Fatalf("good chunk address mismatch after recovery")
	}

	// 8. The unreadable chunk must have been pruned from the index.
	_, err = st2.Storage().ChunkStore().Get(ctx, badChunk.Address())
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unreadable chunk, got: %v", err)
	}
}
