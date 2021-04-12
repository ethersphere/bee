// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestDB_ReserveGC_AllOutOfRadius tests that when all chunks fall outside of
// batch radius, all end up in the cache and that gc size eventually
// converges to the correct value.
func TestDB_ReserveGC_AllOutOfRadius(t *testing.T) {
	chunkCount := 150

	var closed chan struct{}
	testHookCollectGarbageChan := make(chan uint64)
	t.Cleanup(setTestHookCollectGarbage(func(collectedCount uint64) {
		select {
		case testHookCollectGarbageChan <- collectedCount:
		case <-closed:
		}
	}))

	db := newTestDB(t, &Options{
		Capacity: 100,
	})
	closed = db.close

	addrs := make([]swarm.Address, 0)

	for i := 0; i < chunkCount; i++ {
		ch := generateTestRandomChunkAt(swarm.NewAddress(db.baseKey), 2).WithBatch(3, 3)
		err := db.UnreserveBatch(ch.Stamp().BatchID(), 4)
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.Put(context.Background(), storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}
		err = db.Set(context.Background(), storage.ModeSetSync, ch.Address())
		if err != nil {
			t.Fatal(err)
		}

		addrs = append(addrs, ch.Address())
	}

	gcTarget := db.gcTarget()

	for {
		select {
		case <-testHookCollectGarbageChan:
		case <-time.After(10 * time.Second):
			t.Fatal("collect garbage timeout")
		}
		gcSize, err := db.gcSize.Get()
		if err != nil {
			t.Fatal(err)
		}
		if gcSize == gcTarget {
			break
		}
	}

	t.Run("pull index count", newItemsCountTest(db.pullIndex, int(gcTarget)))

	t.Run("postage chunks index count", newItemsCountTest(db.postageChunksIndex, int(gcTarget)))

	t.Run("gc index count", newItemsCountTest(db.gcIndex, int(gcTarget)))

	t.Run("gc size", newIndexGCSizeTest(db))

	// the first synced chunk should be removed
	t.Run("get the first synced chunk", func(t *testing.T) {
		_, err := db.Get(context.Background(), storage.ModeGetRequest, addrs[0])
		if !errors.Is(err, storage.ErrNotFound) {
			t.Errorf("got error %v, want %v", err, storage.ErrNotFound)
		}
	})

	t.Run("only first inserted chunks should be removed", func(t *testing.T) {
		for i := 0; i < (chunkCount - int(gcTarget)); i++ {
			_, err := db.Get(context.Background(), storage.ModeGetRequest, addrs[i])
			if !errors.Is(err, storage.ErrNotFound) {
				t.Errorf("got error %v, want %v", err, storage.ErrNotFound)
			}
		}
	})

	// last synced chunk should not be removed
	t.Run("get most recent synced chunk", func(t *testing.T) {
		_, err := db.Get(context.Background(), storage.ModeGetRequest, addrs[len(addrs)-1])
		if err != nil {
			t.Fatal(err)
		}
	})
}

// TestDB_ReserveGC_AllWithinRadius tests that when all chunks fall within
// batch radius, none get collected.
func TestDB_ReserveGC_AllWithinRadius(t *testing.T) {
	chunkCount := 150

	var closed chan struct{}
	testHookCollectGarbageChan := make(chan uint64)
	t.Cleanup(setTestHookCollectGarbage(func(collectedCount uint64) {
		select {
		case testHookCollectGarbageChan <- collectedCount:
		case <-closed:
		}
	}))

	db := newTestDB(t, &Options{
		Capacity: 100,
	})
	closed = db.close

	addrs := make([]swarm.Address, 0)

	for i := 0; i < chunkCount; i++ {
		ch := generateTestRandomChunkAt(swarm.NewAddress(db.baseKey), 2).WithBatch(2, 3)
		err := db.UnreserveBatch(ch.Stamp().BatchID(), 2)
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.Put(context.Background(), storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}
		err = db.Set(context.Background(), storage.ModeSetSync, ch.Address())
		if err != nil {
			t.Fatal(err)
		}

		addrs = append(addrs, ch.Address())
	}

	select {
	case <-testHookCollectGarbageChan:
		t.Fatal("gc ran but shouldnt have")
	case <-time.After(1 * time.Second):
	}

	t.Run("pull index count", newItemsCountTest(db.pullIndex, chunkCount))

	t.Run("postage chunks index count", newItemsCountTest(db.postageChunksIndex, chunkCount))

	t.Run("gc index count", newItemsCountTest(db.gcIndex, 0))

	t.Run("gc size", newIndexGCSizeTest(db))

	t.Run("all chunks should be accessible", func(t *testing.T) {
		for _, a := range addrs {
			_, err := db.Get(context.Background(), storage.ModeGetRequest, a)
			if err != nil {
				t.Errorf("got error %v, want none", err)
			}
		}
	})
}

// TestDB_ReserveGC_Unreserve tests that after calling UnreserveBatch
// with a certain radius change, the correct chunks get put into the
// GC index and eventually get garbage collected.
// batch radius, none get collected.
func TestDB_ReserveGC_Unreserve(t *testing.T) {
	chunkCount := 150

	var closed chan struct{}
	testHookCollectGarbageChan := make(chan uint64)
	t.Cleanup(setTestHookCollectGarbage(func(collectedCount uint64) {
		select {
		case testHookCollectGarbageChan <- collectedCount:
		case <-closed:
		}
	}))

	db := newTestDB(t, &Options{
		Capacity: 100,
	})
	closed = db.close

	// put the first chunkCount chunks within radius
	for i := 0; i < chunkCount; i++ {
		ch := generateTestRandomChunkAt(swarm.NewAddress(db.baseKey), 2).WithBatch(2, 3)
		err := db.UnreserveBatch(ch.Stamp().BatchID(), 2)
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.Put(context.Background(), storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}
		err = db.Set(context.Background(), storage.ModeSetSync, ch.Address())
		if err != nil {
			t.Fatal(err)
		}
	}

	var gcChs []swarm.Chunk
	for i := 0; i < 100; i++ {
		gcch := generateTestRandomChunkAt(swarm.NewAddress(db.baseKey), 2).WithBatch(2, 3)
		err := db.UnreserveBatch(gcch.Stamp().BatchID(), 2)
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.Put(context.Background(), storage.ModePutUpload, gcch)
		if err != nil {
			t.Fatal(err)
		}
		err = db.Set(context.Background(), storage.ModeSetSync, gcch.Address())
		if err != nil {
			t.Fatal(err)
		}
		gcChs = append(gcChs, gcch)
	}

	// radius increases from 2 to 3, chunk is in PO 2, therefore it should be
	// GCd

	for _, ch := range gcChs {
		err := db.UnreserveBatch(ch.Stamp().BatchID(), 3)
		if err != nil {
			t.Fatal(err)
		}
	}

	gcTarget := db.gcTarget()

	for {
		select {
		case <-testHookCollectGarbageChan:
		case <-time.After(10 * time.Second):
			t.Fatal("collect garbage timeout")
		}
		gcSize, err := db.gcSize.Get()
		if err != nil {
			t.Fatal(err)
		}
		if gcSize == gcTarget {
			break
		}
	}
	t.Run("pull index count", newItemsCountTest(db.pullIndex, chunkCount+90))

	t.Run("postage chunks index count", newItemsCountTest(db.postageChunksIndex, chunkCount+90))

	t.Run("gc index count", newItemsCountTest(db.gcIndex, 90))

	t.Run("gc size", newIndexGCSizeTest(db))

	t.Run("first ten unreserved chunks should not be accessible", func(t *testing.T) {
		for _, ch := range gcChs[:10] {
			_, err := db.Get(context.Background(), storage.ModeGetRequest, ch.Address())
			if err == nil {
				t.Error("got no error, want NotFound")
			}
		}
	})

	t.Run("the rest should be accessible", func(t *testing.T) {
		for _, ch := range gcChs[10:] {
			_, err := db.Get(context.Background(), storage.ModeGetRequest, ch.Address())
			if err != nil {
				t.Errorf("got error %v but want none", err)
			}
		}
	})
}
