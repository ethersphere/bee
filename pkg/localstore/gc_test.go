// Copyright 2018 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package localstore

import (
	"bytes"
	"context"
	"errors"
	"io/ioutil"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/syndtr/goleveldb/leveldb"
)

// TestDB_collectGarbageWorker tests garbage collection runs
// by uploading and syncing a number of chunks.
func TestDB_collectGarbageWorker(t *testing.T) {
	testDBCollectGarbageWorker(t)
}

// TestDB_collectGarbageWorker_multipleBatches tests garbage
// collection runs by uploading and syncing a number of
// chunks by having multiple smaller batches.
func TestDB_collectGarbageWorker_multipleBatches(t *testing.T) {
	// lower the maximal number of chunks in a single
	// gc batch to ensure multiple batches.
	defer func(s uint64) { gcBatchSize = s }(gcBatchSize)
	gcBatchSize = 2

	testDBCollectGarbageWorker(t)
}

// testDBCollectGarbageWorker is a helper test function to test
// garbage collection runs by uploading and syncing a number of chunks.
func testDBCollectGarbageWorker(t *testing.T) {

	chunkCount := 150

	var closed chan struct{}
	testHookCollectGarbageChan := make(chan uint64)
	t.Cleanup(setTestHookCollectGarbage(func(collectedCount uint64) {
		// don't trigger if we haven't collected anything - this may
		// result in a race condition when we inspect the gcsize below,
		// causing the database to shut down while the cleanup to happen
		// before the correct signal has been communicated here.
		if collectedCount == 0 {
			return
		}
		select {
		case testHookCollectGarbageChan <- collectedCount:
		case <-closed:
		}
	}))

	t.Cleanup(setWithinRadiusFunc(func(_ *DB, _ shed.Item) bool { return false }))
	db := newTestDB(t, &Options{
		Capacity: 100,
	})
	closed = db.close

	addrs := make([]swarm.Address, chunkCount)
	ctx := context.Background()
	// upload random chunks
	for i := 0; i < chunkCount; i++ {
		ch := generateTestRandomChunk()
		// call unreserve on the batch with radius 0 so that
		// localstore is aware of the batch and the chunk can
		// be inserted into the database
		unreserveChunkBatch(t, db, 0, ch)
		_, err := db.Put(ctx, storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}

		err = db.Set(ctx, storage.ModeSetSync, ch.Address())
		if err != nil {
			t.Fatal(err)
		}

		addrs[i] = ch.Address()
	}

	gcTarget := db.gcTarget()

	for {
		select {
		case <-testHookCollectGarbageChan:
		case <-time.After(10 * time.Second):
			t.Error("collect garbage timeout")
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

// Pin a file, upload chunks to go past the gc limit to trigger GC,
// check if the pinned files are still around and removed from gcIndex
func TestPinGC(t *testing.T) {
	chunkCount := 150
	pinChunksCount := 50
	cacheCapacity := uint64(100)

	var closed chan struct{}
	testHookCollectGarbageChan := make(chan uint64)
	t.Cleanup(setTestHookCollectGarbage(func(collectedCount uint64) {
		// don't trigger if we haven't collected anything - this may
		// result in a race condition when we inspect the gcsize below,
		// causing the database to shut down while the cleanup to happen
		// before the correct signal has been communicated here.
		if collectedCount == 0 {
			return
		}

		select {
		case testHookCollectGarbageChan <- collectedCount:
		case <-closed:
		}
	}))
	t.Cleanup(setWithinRadiusFunc(func(_ *DB, _ shed.Item) bool { return false }))

	db := newTestDB(t, &Options{
		Capacity: cacheCapacity,
	})
	closed = db.close

	addrs := make([]swarm.Address, 0)
	pinAddrs := make([]swarm.Address, 0)

	// upload random chunks
	for i := 0; i < chunkCount; i++ {
		ch := generateTestRandomChunk()
		// call unreserve on the batch with radius 0 so that
		// localstore is aware of the batch and the chunk can
		// be inserted into the database
		unreserveChunkBatch(t, db, 0, ch)

		mode := storage.ModePutUpload
		if i < pinChunksCount {
			mode = storage.ModePutUploadPin
			pinAddrs = append(pinAddrs, ch.Address())
		}

		_, err := db.Put(context.Background(), mode, ch)
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
	t.Log(gcTarget)
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

	t.Run("pin Index count", newItemsCountTest(db.pinIndex, pinChunksCount))

	t.Run("pull index count", newItemsCountTest(db.pullIndex, int(gcTarget)+pinChunksCount))

	t.Run("gc index count", newItemsCountTest(db.gcIndex, int(gcTarget)))

	t.Run("gc size", newIndexGCSizeTest(db))

	t.Run("pinned chunk not in gc Index", func(t *testing.T) {
		err := db.gcIndex.Iterate(func(item shed.Item) (stop bool, err error) {
			for _, pinHash := range pinAddrs {
				if bytes.Equal(pinHash.Bytes(), item.Address) {
					t.Fatal("pin chunk present in gcIndex")
				}
			}
			return false, nil
		}, nil)
		if err != nil {
			t.Fatal("could not iterate gcIndex")
		}
	})

	t.Run("pinned chunks exists", func(t *testing.T) {
		for _, hash := range pinAddrs {
			_, err := db.Get(context.Background(), storage.ModeGetRequest, hash)
			if err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("first chunks after pinned chunks should be removed", func(t *testing.T) {
		for i := pinChunksCount; i < (int(cacheCapacity) - int(gcTarget)); i++ {
			_, err := db.Get(context.Background(), storage.ModeGetRequest, addrs[i])
			if !errors.Is(err, leveldb.ErrNotFound) {
				t.Fatal(err)
			}
		}
	})
}

// Upload chunks, pin those chunks, add to GC after it is pinned
// check if the pinned files are still around
func TestGCAfterPin(t *testing.T) {

	chunkCount := 50

	db := newTestDB(t, &Options{
		Capacity: 100,
	})

	pinAddrs := make([]swarm.Address, 0)

	// upload random chunks
	for i := 0; i < chunkCount; i++ {
		ch := generateTestRandomChunk()
		// call unreserve on the batch with radius 0 so that
		// localstore is aware of the batch and the chunk can
		// be inserted into the database
		unreserveChunkBatch(t, db, 0, ch)

		_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}

		// Pin before adding to GC in ModeSetSync
		err = db.Set(context.Background(), storage.ModeSetPin, ch.Address())
		if err != nil {
			t.Fatal(err)
		}
		pinAddrs = append(pinAddrs, ch.Address())

		err = db.Set(context.Background(), storage.ModeSetSync, ch.Address())
		if err != nil {
			t.Fatal(err)
		}
	}

	t.Run("pin Index count", newItemsCountTest(db.pinIndex, chunkCount))

	t.Run("gc index count", newItemsCountTest(db.gcIndex, int(0)))

	for _, hash := range pinAddrs {
		_, err := db.Get(context.Background(), storage.ModeGetRequest, hash)
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestDB_collectGarbageWorker_withRequests is a helper test function
// to test garbage collection runs by uploading, syncing and
// requesting a number of chunks.
func TestDB_collectGarbageWorker_withRequests(t *testing.T) {
	testHookCollectGarbageChan := make(chan uint64)
	defer setTestHookCollectGarbage(func(collectedCount uint64) {
		// don't trigger if we haven't collected anything - this may
		// result in a race condition when we inspect the gcsize below,
		// causing the database to shut down while the cleanup to happen
		// before the correct signal has been communicated here.
		if collectedCount == 0 {
			return
		}
		testHookCollectGarbageChan <- collectedCount
	})()

	t.Cleanup(setWithinRadiusFunc(func(_ *DB, _ shed.Item) bool { return false }))

	db := newTestDB(t, &Options{
		Capacity: 100,
	})

	addrs := make([]swarm.Address, 0)

	// upload random chunks just up to the capacity
	for i := 0; i < int(db.cacheCapacity)-1; i++ {
		ch := generateTestRandomChunk()
		// call unreserve on the batch with radius 0 so that
		// localstore is aware of the batch and the chunk can
		// be inserted into the database
		unreserveChunkBatch(t, db, 0, ch)

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

	// set update gc test hook to signal when
	// update gc goroutine is done by closing
	// testHookUpdateGCChan channel
	testHookUpdateGCChan := make(chan struct{})
	resetTestHookUpdateGC := setTestHookUpdateGC(func() {
		close(testHookUpdateGCChan)
	})

	// request the oldest synced chunk
	// to prioritize it in the gc index
	// not to be collected
	_, err := db.Get(context.Background(), storage.ModeGetRequest, addrs[0])
	if err != nil {
		t.Fatal(err)
	}

	// wait for update gc goroutine to finish for garbage
	// collector to be correctly triggered after the last upload
	select {
	case <-testHookUpdateGCChan:
	case <-time.After(10 * time.Second):
		t.Fatal("updateGC was not called after getting chunk with ModeGetRequest")
	}

	// no need to wait for update gc hook anymore
	resetTestHookUpdateGC()

	// upload and sync another chunk to trigger
	// garbage collection
	ch := generateTestRandomChunk()
	// call unreserve on the batch with radius 0 so that
	// localstore is aware of the batch and the chunk can
	// be inserted into the database
	unreserveChunkBatch(t, db, 0, ch)

	_, err = db.Put(context.Background(), storage.ModePutUpload, ch)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Set(context.Background(), storage.ModeSetSync, ch.Address())
	if err != nil {
		t.Fatal(err)
	}
	addrs = append(addrs, ch.Address())

	// wait for garbage collection

	gcTarget := db.gcTarget()

	var totalCollectedCount uint64
	for {
		select {
		case c := <-testHookCollectGarbageChan:
			totalCollectedCount += c
		case <-time.After(10 * time.Second):
			t.Error("collect garbage timeout")
		}
		gcSize, err := db.gcSize.Get()
		if err != nil {
			t.Fatal(err)
		}
		if gcSize == gcTarget {
			break
		}
	}

	wantTotalCollectedCount := uint64(len(addrs)) - gcTarget
	if totalCollectedCount != wantTotalCollectedCount {
		t.Errorf("total collected chunks %v, want %v", totalCollectedCount, wantTotalCollectedCount)
	}

	t.Run("pull index count", newItemsCountTest(db.pullIndex, int(gcTarget)))

	t.Run("gc index count", newItemsCountTest(db.gcIndex, int(gcTarget)))

	t.Run("gc size", newIndexGCSizeTest(db))

	// requested chunk should not be removed
	t.Run("get requested chunk", func(t *testing.T) {
		_, err := db.Get(context.Background(), storage.ModeGetRequest, addrs[0])
		if err != nil {
			t.Fatal(err)
		}
	})

	// the second synced chunk should be removed
	t.Run("get gc-ed chunk", func(t *testing.T) {
		_, err := db.Get(context.Background(), storage.ModeGetRequest, addrs[1])
		if !errors.Is(err, storage.ErrNotFound) {
			t.Errorf("got error %v, want %v", err, storage.ErrNotFound)
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

// TestDB_gcSize checks if gcSize has a correct value after
// database is initialized with existing data.
func TestDB_gcSize(t *testing.T) {
	dir, err := ioutil.TempDir("", "localstore-stored-gc-size")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	baseKey := make([]byte, 32)
	if _, err := rand.Read(baseKey); err != nil {
		t.Fatal(err)
	}
	logger := logging.New(ioutil.Discard, 0)
	db, err := New(dir, baseKey, nil, logger)
	if err != nil {
		t.Fatal(err)
	}

	count := 100

	for i := 0; i < count; i++ {
		ch := generateTestRandomChunk()
		// call unreserve on the batch with radius 0 so that
		// localstore is aware of the batch and the chunk can
		// be inserted into the database
		unreserveChunkBatch(t, db, 0, ch)

		_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}

		err = db.Set(context.Background(), storage.ModeSetSync, ch.Address())
		if err != nil {
			t.Fatal(err)
		}
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = New(dir, baseKey, nil, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	t.Run("gc index size", newIndexGCSizeTest(db))
}

// setTestHookCollectGarbage sets testHookCollectGarbage and
// returns a function that will reset it to the
// value before the change.
func setTestHookCollectGarbage(h func(collectedCount uint64)) (reset func()) {
	current := testHookCollectGarbage
	reset = func() { testHookCollectGarbage = current }
	testHookCollectGarbage = h
	return reset
}

func setWithinRadiusFunc(h func(*DB, shed.Item) bool) (reset func()) {
	current := withinRadiusFn
	reset = func() { withinRadiusFn = current }
	withinRadiusFn = h
	return reset
}

// TestSetTestHookCollectGarbage tests if setTestHookCollectGarbage changes
// testHookCollectGarbage function correctly and if its reset function
// resets the original function.
func TestSetTestHookCollectGarbage(t *testing.T) {
	// Set the current function after the test finishes.
	defer func(h func(collectedCount uint64)) { testHookCollectGarbage = h }(testHookCollectGarbage)

	// expected value for the unchanged function
	original := 1
	// expected value for the changed function
	changed := 2

	// this variable will be set with two different functions
	var got int

	// define the original (unchanged) functions
	testHookCollectGarbage = func(_ uint64) {
		got = original
	}

	// set got variable
	testHookCollectGarbage(0)

	// test if got variable is set correctly
	if got != original {
		t.Errorf("got hook value %v, want %v", got, original)
	}

	// set the new function
	reset := setTestHookCollectGarbage(func(_ uint64) {
		got = changed
	})

	// set got variable
	testHookCollectGarbage(0)

	// test if got variable is set correctly to changed value
	if got != changed {
		t.Errorf("got hook value %v, want %v", got, changed)
	}

	// set the function to the original one
	reset()

	// set got variable
	testHookCollectGarbage(0)

	// test if got variable is set correctly to original value
	if got != original {
		t.Errorf("got hook value %v, want %v", got, original)
	}
}

func TestPinAfterMultiGC(t *testing.T) {
	t.Cleanup(setWithinRadiusFunc(func(_ *DB, _ shed.Item) bool { return false }))
	db := newTestDB(t, &Options{
		Capacity: 10,
	})

	pinnedChunks := make([]swarm.Address, 0)

	// upload random chunks above cache capacity to see if chunks are still pinned
	for i := 0; i < 20; i++ {
		ch := generateTestRandomChunk()
		// call unreserve on the batch with radius 0 so that
		// localstore is aware of the batch and the chunk can
		// be inserted into the database
		unreserveChunkBatch(t, db, 0, ch)

		_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}
		err = db.Set(context.Background(), storage.ModeSetSync, ch.Address())
		if err != nil {
			t.Fatal(err)
		}

		if len(pinnedChunks) < 10 {
			rch := generateAndPinAChunk(t, db)
			pinnedChunks = append(pinnedChunks, rch.Address())
		}
	}
	for i := 0; i < 20; i++ {
		ch := generateTestRandomChunk()
		// call unreserve on the batch with radius 0 so that
		// localstore is aware of the batch and the chunk can
		// be inserted into the database
		unreserveChunkBatch(t, db, 0, ch)

		_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}
		err = db.Set(context.Background(), storage.ModeSetSync, ch.Address())
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 20; i++ {
		ch := generateTestRandomChunk()
		// call unreserve on the batch with radius 0 so that
		// localstore is aware of the batch and the chunk can
		// be inserted into the database
		unreserveChunkBatch(t, db, 0, ch)

		_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}
		err = db.Set(context.Background(), storage.ModeSetSync, ch.Address())
		if err != nil {
			t.Fatal(err)
		}
	}

	t.Run("pin Index count", newItemsCountTest(db.pinIndex, len(pinnedChunks)))

	// Check if all the pinned chunks are present in the data DB
	for _, addr := range pinnedChunks {
		outItem := shed.Item{
			Address: addr.Bytes(),
		}
		gotChunk, err := db.Get(context.Background(), storage.ModeGetRequest, swarm.NewAddress(outItem.Address))
		if err != nil {
			t.Fatal(err)
		}
		if !gotChunk.Address().Equal(swarm.NewAddress(addr.Bytes())) {
			t.Fatal("Pinned chunk is not equal to got chunk")
		}
	}

}

func generateAndPinAChunk(t *testing.T, db *DB) swarm.Chunk {
	// Create a chunk and pin it
	ch := generateTestRandomChunk()

	_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
	if err != nil {
		t.Fatal(err)
	}
	// call unreserve on the batch with radius 0 so that
	// localstore is aware of the batch and the chunk can
	// be inserted into the database
	unreserveChunkBatch(t, db, 0, ch)

	err = db.Set(context.Background(), storage.ModeSetPin, ch.Address())
	if err != nil {
		t.Fatal(err)
	}
	err = db.Set(context.Background(), storage.ModeSetSync, ch.Address())
	if err != nil {
		t.Fatal(err)
	}
	return ch
}

func TestPinSyncAndAccessPutSetChunkMultipleTimes(t *testing.T) {
	var closed chan struct{}
	testHookCollectGarbageChan := make(chan uint64)
	t.Cleanup(setTestHookCollectGarbage(func(collectedCount uint64) {
		// don't trigger if we haven't collected anything - this may
		// result in a race condition when we inspect the gcsize below,
		// causing the database to shut down while the cleanup to happen
		// before the correct signal has been communicated here.
		if collectedCount == 0 {
			return
		}
		select {
		case testHookCollectGarbageChan <- collectedCount:
		case <-closed:
		}
	}))
	db := newTestDB(t, &Options{
		Capacity: 10,
	})
	closed = db.close

	pinnedChunks := addRandomChunks(t, 5, db, true)
	rand1Chunks := addRandomChunks(t, 15, db, false)
	for _, ch := range pinnedChunks {
		_, err := db.Get(context.Background(), storage.ModeGetRequest, ch.Address())
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, ch := range rand1Chunks {
		_, err := db.Get(context.Background(), storage.ModeGetRequest, ch.Address())
		if err != nil {
			// ignore the chunks that are GCd
			continue
		}
	}

	rand2Chunks := addRandomChunks(t, 20, db, false)
	for _, ch := range rand2Chunks {
		_, err := db.Get(context.Background(), storage.ModeGetRequest, ch.Address())
		if err != nil {
			// ignore the chunks that are GCd
			continue
		}
	}

	rand3Chunks := addRandomChunks(t, 20, db, false)

	for _, ch := range rand3Chunks {
		_, err := db.Get(context.Background(), storage.ModeGetRequest, ch.Address())
		if err != nil {
			// ignore the chunks that are GCd
			continue
		}
	}

	// check if the pinned chunk is present after GC
	for _, ch := range pinnedChunks {
		gotChunk, err := db.Get(context.Background(), storage.ModeGetRequest, ch.Address())
		if err != nil {
			t.Fatal("Pinned chunk missing ", err)
		}
		if !gotChunk.Address().Equal(ch.Address()) {
			t.Fatal("Pinned chunk address is not equal to got chunk")
		}

		if !bytes.Equal(gotChunk.Data(), ch.Data()) {
			t.Fatal("Pinned chunk data is not equal to got chunk")
		}
	}

}

func addRandomChunks(t *testing.T, count int, db *DB, pin bool) []swarm.Chunk {
	var chunks []swarm.Chunk
	for i := 0; i < count; i++ {
		ch := generateTestRandomChunk()
		unreserveChunkBatch(t, db, 0, ch)

		_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}
		if pin {
			err = db.Set(context.Background(), storage.ModeSetPin, ch.Address())
			if err != nil {
				t.Fatal(err)
			}
			err = db.Set(context.Background(), storage.ModeSetSync, ch.Address())
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.Get(context.Background(), storage.ModeGetRequest, ch.Address())
			if err != nil {
				t.Fatal(err)
			}
		} else {
			// Non pinned chunks could be GC'd by the time they reach here.
			// so it is okay to ignore the error
			_ = db.Set(context.Background(), storage.ModeSetSync, ch.Address())
			_, _ = db.Get(context.Background(), storage.ModeGetRequest, ch.Address())
		}
		chunks = append(chunks, ch)
	}
	return chunks
}

// TestGC_NoEvictDirty checks that the garbage collection
// does not evict chunks that are marked as dirty while the gc
// is running.
func TestGC_NoEvictDirty(t *testing.T) {
	// lower the maximal number of chunks in a single
	// gc batch to ensure multiple batches.
	t.Cleanup(setWithinRadiusFunc(func(_ *DB, _ shed.Item) bool { return false }))
	defer func(s uint64) { gcBatchSize = s }(gcBatchSize)
	gcBatchSize = 1

	chunkCount := 10

	db := newTestDB(t, &Options{
		Capacity: 10,
	})

	testHookCollectGarbageChan := make(chan uint64)
	t.Cleanup(setTestHookCollectGarbage(func(collectedCount uint64) {
		// don't trigger if we haven't collected anything - this may
		// result in a race condition when we inspect the gcsize below,
		// causing the database to shut down while the cleanup to happen
		// before the correct signal has been communicated here.
		if collectedCount == 0 {
			return
		}
		select {
		case testHookCollectGarbageChan <- collectedCount:
		case <-db.close:
		}
	}))

	dirtyChan := make(chan struct{})
	incomingChan := make(chan struct{})
	t.Cleanup(setTestHookGCIteratorDone(func() {
		incomingChan <- struct{}{}
		<-dirtyChan
	}))
	defer close(incomingChan)
	addrs := make([]swarm.Address, 0)
	mtx := new(sync.Mutex)
	online := make(chan struct{})
	go func() {
		close(online) // make sure this is scheduled, otherwise test might flake
		i := 0
		for range incomingChan {
			// set a chunk to be updated in gc, resulting
			// in a removal from the gc round. but don't do this
			// for all chunks!
			if i < 2 {
				mtx.Lock()
				_, err := db.Get(context.Background(), storage.ModeGetRequest, addrs[i])
				mtx.Unlock()
				if err != nil {
					t.Error(err)
				}
				i++
				// we sleep so that the async update to gc index
				// happens and that the dirtyAddresses get updated
				time.Sleep(100 * time.Millisecond)
			}
			dirtyChan <- struct{}{}
		}
	}()
	<-online
	// upload random chunks
	for i := 0; i < chunkCount; i++ {
		ch := generateTestRandomChunk()
		unreserveChunkBatch(t, db, 0, ch)

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
		mtx.Unlock()
	}

	gcTarget := db.gcTarget()
	for {
		select {
		case <-testHookCollectGarbageChan:
		case <-time.After(10 * time.Second):
			t.Error("collect garbage timeout")
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

	t.Run("gc index count", newItemsCountTest(db.gcIndex, int(gcTarget)))

	t.Run("gc size", newIndexGCSizeTest(db))

	// the first synced chunk should be removed
	t.Run("get the first two chunks, third is gone", func(t *testing.T) {
		_, err := db.Get(context.Background(), storage.ModeGetRequest, addrs[0])
		if err != nil {
			t.Error("got error but expected none")
		}
		_, err = db.Get(context.Background(), storage.ModeGetRequest, addrs[1])
		if err != nil {
			t.Error("got error but expected none")
		}
		_, err = db.Get(context.Background(), storage.ModeGetRequest, addrs[2])
		if !errors.Is(err, storage.ErrNotFound) {
			t.Errorf("expected err not found but got %v", err)
		}
	})

	t.Run("only later inserted chunks should be removed", func(t *testing.T) {
		for i := 2; i < (chunkCount - int(gcTarget)); i++ {
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

// setTestHookGCIteratorDone sets testHookGCIteratorDone and
// returns a function that will reset it to the
// value before the change.
func setTestHookGCIteratorDone(h func()) (reset func()) {
	current := testHookGCIteratorDone
	reset = func() { testHookGCIteratorDone = current }
	testHookGCIteratorDone = h
	return reset
}

func unreserveChunkBatch(t *testing.T, db *DB, radius uint8, chs ...swarm.Chunk) {
	t.Helper()
	for _, ch := range chs {
		err := db.UnreserveBatch(ch.Stamp().BatchID(), radius)
		if err != nil {
			t.Fatal(err)
		}
	}
}
