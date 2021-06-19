// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/shed"
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
	t.Cleanup(setWithinRadiusFunc(func(*DB, shed.Item) bool { return false }))

	db := newTestDB(t, &Options{
		Capacity:        100,
		ReserveCapacity: 200,
	})
	closed = db.close

	addrs := make([]swarm.Address, 0)

	for i := 0; i < chunkCount; i++ {
		ch := generateTestRandomChunkAt(swarm.NewAddress(db.baseKey), 2).WithBatch(5, 3, 2, false)
		_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
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

	// postageRadiusIndex gets removed only when the batches are called with evict on MaxPO+1
	// therefore, the expected index count here is larger than one would expect.
	t.Run("postage radius index count", newItemsCountTest(db.postageRadiusIndex, 0))

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
	var (
		batchIDs        [][]byte
		unreserveCalled bool
		mtx             sync.Mutex
	)
	unres := func(f postage.UnreserveIteratorFn) error {
		mtx.Lock()
		defer mtx.Unlock()
		unreserveCalled = true
		for i := 0; i < len(batchIDs); i++ {
			// pop an element from batchIDs, call the Unreserve
			item := batchIDs[i]
			// here we mock the behavior of the batchstore
			// that would call the localstore back with the
			// batch IDs and the radiuses from the FIFO queue
			stop, err := f(item, 4)
			if err != nil {
				return err
			}
			if stop {
				return nil
			}
		}
		return nil
	}

	db := newTestDB(t, &Options{
		Capacity:        100,
		ReserveCapacity: 151,
		UnreserveFunc:   unres,
	})
	closed = db.close

	addrs := make([]swarm.Address, 0)

	for i := 0; i < chunkCount; i++ {
		ch := generateTestRandomChunkAt(swarm.NewAddress(db.baseKey), 2).WithBatch(2, 3, 2, false)
		_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}
		err = db.Set(context.Background(), storage.ModeSetSync, ch.Address())
		if err != nil {
			t.Fatal(err)
		}
		mtx.Lock()
		addrs = append(addrs, ch.Address())
		batchIDs = append(batchIDs, ch.Stamp().BatchID())
		mtx.Unlock()
	}

	select {
	case <-testHookCollectGarbageChan:
		t.Fatal("gc ran but shouldnt have")
	case <-time.After(1 * time.Second):
	}

	t.Run("pull index count", newItemsCountTest(db.pullIndex, chunkCount))

	t.Run("postage chunks index count", newItemsCountTest(db.postageChunksIndex, chunkCount))

	t.Run("postage radius index count", newItemsCountTest(db.postageRadiusIndex, 0))

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
	mtx.Lock()
	defer mtx.Unlock()
	if unreserveCalled {
		t.Fatal("unreserveCalled but should not have")
	}
}

// TestDB_ReserveGC_Unreserve tests that after calling UnreserveBatch
// with a certain radius change, the correct chunks get put into the
// GC index and eventually get garbage collected.
func TestDB_ReserveGC_Unreserve(t *testing.T) {
	chunkCount := 100

	var closed chan struct{}
	testHookCollectGarbageChan := make(chan uint64)
	testHookEvictChan := make(chan uint64)
	t.Cleanup(setTestHookCollectGarbage(func(collectedCount uint64) {
		select {
		case testHookCollectGarbageChan <- collectedCount:
		case <-closed:
		}
	}))
	t.Cleanup(setTestHookEviction(func(collectedCount uint64) {
		select {
		case testHookEvictChan <- collectedCount:
		case <-closed:
		}
	}))

	var (
		mtx      sync.Mutex
		batchIDs [][]byte
		addrs    []swarm.Address
	)

	unres := func(f postage.UnreserveIteratorFn) error {
		mtx.Lock()
		defer mtx.Unlock()
		for i := 0; i < len(batchIDs); i++ {
			// pop an element from batchIDs, call the Unreserve
			item := batchIDs[i]
			// here we mock the behavior of the batchstore
			// that would call the localstore back with the
			// batch IDs and the radiuses from the FIFO queue
			stop, err := f(item, 2)
			if err != nil {
				return err
			}
			if stop {
				return nil
			}
			stop, err = f(item, 4)
			if err != nil {
				return err
			}
			if stop {
				return nil
			}
		}
		batchIDs = nil
		return nil
	}

	db := newTestDB(t, &Options{
		Capacity: 100,
		// once reaching 150 in the reserve, we will evict
		// half the size of the cache from the reserve, so 50 chunks
		ReserveCapacity: 100,
		UnreserveFunc:   unres,
	})
	closed = db.close

	// put chunksCount chunks within radius. this
	// will cause reserve eviction of 10 chunks into
	// the cache. gc of the cache is still not triggered
	for i := 0; i < chunkCount; i++ {
		ch := generateTestRandomChunkAt(swarm.NewAddress(db.baseKey), 2).WithBatch(2, 3, 2, false)
		_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}
		err = db.Set(context.Background(), storage.ModeSetSync, ch.Address())
		if err != nil {
			t.Fatal(err)
		}
		mtx.Lock()
		batchIDs = append(batchIDs, ch.Stamp().BatchID())
		addrs = append(addrs, ch.Address())
		mtx.Unlock()
	}

	// wait for the first eviction to finish, otherwise
	// we collect some of the next chunks that get added
	// which results in inconsistencies
	evictTarget := db.reserveEvictionTarget()

	for {
		select {
		case <-testHookEvictChan:
		case <-time.After(10 * time.Second):
			t.Fatal("collect garbage timeout")
		}
		resSize, err := db.reserveSize.Get()
		if err != nil {
			t.Fatal(err)
		}
		if resSize == evictTarget {
			break
		}
	}

	// insert another 90, this will trigger gc
	for i := 0; i < 90; i++ {
		ch := generateTestRandomChunkAt(swarm.NewAddress(db.baseKey), 2).WithBatch(2, 3, 2, false)
		_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}
		err = db.Set(context.Background(), storage.ModeSetSync, ch.Address())
		if err != nil {
			t.Fatal(err)
		}
		mtx.Lock()
		batchIDs = append(batchIDs, ch.Stamp().BatchID())
		addrs = append(addrs, ch.Address())
		mtx.Unlock()
	}

	for {
		select {
		case <-testHookEvictChan:
		case <-time.After(10 * time.Second):
			t.Fatal("collect garbage timeout")
		}
		resSize, err := db.reserveSize.Get()
		if err != nil {
			t.Fatal(err)
		}
		if resSize == evictTarget {
			break
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
	t.Run("pull index count", newItemsCountTest(db.pullIndex, chunkCount+90-10))

	t.Run("postage chunks index count", newItemsCountTest(db.postageChunksIndex, chunkCount+90-10))

	// postageRadiusIndex gets removed only when the batches are called with evict on MaxPO+1
	// therefore, the expected index count here is larger than one would expect.
	t.Run("postage radius index count", newItemsCountTest(db.postageRadiusIndex, chunkCount))

	t.Run("gc index count", newItemsCountTest(db.gcIndex, 90))

	t.Run("gc size", newIndexGCSizeTest(db))

	t.Run("first ten unreserved chunks should not be accessible", func(t *testing.T) {
		for _, a := range addrs[:10] {
			_, err := db.Get(context.Background(), storage.ModeGetRequest, a)
			if err == nil {
				t.Error("got no error, want NotFound")
			}
		}
	})

	t.Run("the rest should be accessible", func(t *testing.T) {
		for _, a := range addrs[10:] {
			_, err := db.Get(context.Background(), storage.ModeGetRequest, a)
			if err != nil {
				t.Errorf("got error %v but want none", err)
			}
		}
	})
}

// TestDB_ReserveGC_EvictMaxPO tests that when unreserving a batch at
// swarm.MaxPO+1 results in the correct behaviour.
func TestDB_ReserveGC_EvictMaxPO(t *testing.T) {

	var (
		mtx        sync.Mutex
		batchIDs   [][]byte
		addrs      []swarm.Address
		chunkCount = 100

		testHookCollectGarbageChan = make(chan uint64)
		testHookEvictChan          = make(chan uint64)
		closed                     chan struct{}
	)
	t.Cleanup(setTestHookCollectGarbage(func(collectedCount uint64) {
		if collectedCount == 0 {
			return
		}
		select {
		case testHookCollectGarbageChan <- collectedCount:
		case <-closed:
		}
	}))
	t.Cleanup(setTestHookEviction(func(collectedCount uint64) {
		if collectedCount == 0 {
			return
		}
		select {
		case testHookEvictChan <- collectedCount:
		case <-closed:
		}
	}))

	unres := func(f postage.UnreserveIteratorFn) error {
		mtx.Lock()
		defer mtx.Unlock()
		i := 0
		defer func() { batchIDs = batchIDs[i:] }()
		for i = 0; i < len(batchIDs); i++ {
			// pop an element from batchIDs, call the Unreserve
			item := batchIDs[i]
			// here we mock the behavior of the batchstore
			// that would call the localstore back with the
			// batch IDs and the radiuses from the FIFO queue
			stop, err := f(item, 2)
			if err != nil {
				return err
			}
			if stop {
				return nil
			}
			stop, err = f(item, swarm.MaxPO+1)
			if err != nil {
				return err
			}
			if stop {
				return nil
			}
		}
		return nil
	}

	db := newTestDB(t, &Options{
		Capacity: 100,
		// once reaching 100 in the reserve, we will evict
		// half the size of the cache from the reserve, so 50 chunks
		ReserveCapacity: 100,
		UnreserveFunc:   unres,
	})

	closed = db.close

	// put the first chunkCount chunks within radius
	for i := 0; i < chunkCount; i++ {
		ch := generateTestRandomChunkAt(swarm.NewAddress(db.baseKey), 2).WithBatch(2, 3, 2, false)
		_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}
		err = db.Set(context.Background(), storage.ModeSetSync, ch.Address())
		if err != nil {
			t.Fatal(err)
		}
		mtx.Lock()
		batchIDs = append(batchIDs, ch.Stamp().BatchID())
		addrs = append(addrs, ch.Address())
		mtx.Unlock()
	}

	// wait for the first eviction to finish, otherwise
	// we collect some of the next chunks that get added
	// which results in inconsistencies
	evictTarget := db.reserveEvictionTarget()

	for {
		select {
		case <-testHookEvictChan:
		case <-time.After(10 * time.Second):
			t.Fatal("collect garbage timeout")
		}
		resSize, err := db.reserveSize.Get()
		if err != nil {
			t.Fatal(err)
		}
		if resSize == evictTarget {
			break
		}
	}

	// this is zero because we call eviction with max PO on the first 10 batches
	// but the next 90 batches were not called with unreserve yet. this means that
	// although the next 90 chunks exist in the store, their according batch radius
	// still isn't persisted, since the localstore still is not aware of their
	// batch radiuses. the same goes for the check after the gc actually evicts the
	// ten chunks out of the cache (we still expect a zero for postage radius for the
	// same reason)
	t.Run("postage radius index count", newItemsCountTest(db.postageRadiusIndex, 0))

	for i := 0; i < 90; i++ {
		ch := generateTestRandomChunkAt(swarm.NewAddress(db.baseKey), 2).WithBatch(2, 3, 2, false)
		_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}
		err = db.Set(context.Background(), storage.ModeSetSync, ch.Address())
		if err != nil {
			t.Fatal(err)
		}
		mtx.Lock()
		batchIDs = append(batchIDs, ch.Stamp().BatchID())
		addrs = append(addrs, ch.Address())
		mtx.Unlock()
	}
	for {
		select {
		case <-testHookEvictChan:
		case <-time.After(10 * time.Second):
			t.Fatal("collect garbage timeout")
		}
		resSize, err := db.reserveSize.Get()
		if err != nil {
			t.Fatal(err)
		}
		if resSize == evictTarget {
			break
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
	t.Run("pull index count", newItemsCountTest(db.pullIndex, chunkCount+90-10))

	t.Run("postage chunks index count", newItemsCountTest(db.postageChunksIndex, chunkCount+90-10))

	t.Run("postage radius index count", newItemsCountTest(db.postageRadiusIndex, 0))

	t.Run("gc index count", newItemsCountTest(db.gcIndex, 90))

	t.Run("gc size", newIndexGCSizeTest(db))

	t.Run("first ten unreserved chunks should not be accessible", func(t *testing.T) {
		for _, a := range addrs[:10] {
			_, err := db.Get(context.Background(), storage.ModeGetRequest, a)
			if err == nil {
				t.Error("got no error, want NotFound")
			}
		}
	})

	t.Run("the rest should be accessible", func(t *testing.T) {
		for _, a := range addrs[10:] {
			_, err := db.Get(context.Background(), storage.ModeGetRequest, a)
			if err != nil {
				t.Errorf("got error %v but want none", err)
			}
		}
	})
}
