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
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/syndtr/goleveldb/leveldb"
)

// TestGC tests garbage collection runs
// after uploading and syncing a number of chunks.
func TestGC(t *testing.T) {
	t.Run("single round", testGC)
	defer func(s uint64) { gcBatchSize = s }(gcBatchSize)
	gcBatchSize = 2
	t.Run("multiple rounds", testGC)
}

// testGC is a helper test function to test
// garbage collection runs after uploading and syncing a number of chunks.
func testGC(t *testing.T) {
	ctx := context.Background()
	chunkCount := 150

	var closed chan struct{}
	testGCHookChan := make(chan uint64)
	t.Cleanup(setTestHookCollectGarbage(func(collectedCount uint64) {
		select {
		case testGCHookChan <- collectedCount:
		case <-closed:
		}
	}))
	db := newTestDB(t, &Options{
		Capacity: 100,
	})
	closed = db.close

	chunks := generateTestRandomChunks(chunkCount)
	_, err := db.Put(ctx, storage.ModePutSync, chunks...)
	if err != nil {
		t.Fatal(err)
	}

	target := db.gcTarget()
	sizef := func() uint64 {
		size, err := db.gcSize.Get()
		if err != nil {
			t.Fatal(err)
		}
		return size
	}
	size := sizef()
	for target < size {
		t.Logf("GC index: size=%d, target=%d", size, target)
		select {
		case <-testGCHookChan:
		case <-time.After(10 * time.Second):
			t.Fatalf("collect garbage timeout")
		}
		size = sizef()
	}
	t.Run("pull index count", newItemsCountTest(db.pullIndex, int(target)))

	t.Run("gc index count", newItemsCountTest(db.gcIndex, int(target)))

	t.Run("gc size", newIndexGCSizeTest(db))

	// the first synced chunk should be removed
	t.Run("get the first synced chunk", func(t *testing.T) {
		_, err := db.Get(ctx, storage.ModeGetRequest, chunks[0].Address())
		if !errors.Is(err, storage.ErrNotFound) {
			t.Errorf("got error %v, want %v", err, storage.ErrNotFound)
		}
	})

	t.Run("only first inserted chunks should be removed", func(t *testing.T) {
		for i := 0; i < (chunkCount - int(target)); i++ {
			_, err := db.Get(ctx, storage.ModeGetRequest, chunks[i].Address())
			if !errors.Is(err, storage.ErrNotFound) {
				t.Errorf("got error %v, want %v", err, storage.ErrNotFound)
			}
		}
	})

	// last synced chunk should not be removed
	t.Run("get most recent synced chunk", func(t *testing.T) {
		_, err := db.Get(ctx, storage.ModeGetRequest, chunks[len(chunks)-1].Address())
		if err != nil {
			t.Fatal(err)
		}
	})
}

// Pin a file, upload chunks to go past the gc limit to trigger GC,
// check if the pinned files are still around and removed from gcIndex
func TestPinGC(t *testing.T) {
	ctx := context.Background()
	chunkCount := 150
	pinChunksCount := 50
	dbCapacity := uint64(100)

	var closed chan struct{}
	testGCHookChan := make(chan uint64)
	t.Cleanup(setTestHookCollectGarbage(func(collectedCount uint64) {
		select {
		case testGCHookChan <- collectedCount:
		case <-closed:
		}
	}))

	db := newTestDB(t, &Options{
		Capacity: dbCapacity,
	})
	closed = db.close

	addrs := make([]swarm.Address, 0)
	pinAddrs := make([]swarm.Address, 0)

	// upload random chunks
	for cnt := 0; cnt < chunkCount; cnt++ {
		ch := generateTestRandomChunk()
		addr := ch.Address()
		_, err := db.Put(ctx, storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}

		err = db.Set(ctx, storage.ModeSetSync, addr)
		if err != nil {
			t.Fatal(err)
		}

		addrs = append(addrs, addr)

		// Pin the chunks at the beginning to make sure they are not removed by GC
		if cnt < pinChunksCount {
			err = db.Set(ctx, storage.ModeSetPin, addr)
			if err != nil {
				t.Fatal(err)
			}
			item := addressToItem(addr)
			i, err := db.retrievalAccessIndex.Get(item)
			if err != nil {
				t.Fatal(err)
			}
			item.AccessTimestamp = i.AccessTimestamp
			if _, err := db.gcIndex.Get(item); !errors.Is(err, leveldb.ErrNotFound) {
				t.Fatal("pinned chunk present in gcIndex")
			}
			if _, err := db.Get(ctx, storage.ModeGetSync, addr); err != nil {
				t.Fatal(err)
			}
			pinAddrs = append(pinAddrs, addr)
		}
	}
	gcTarget := db.gcTarget()

	for {
		select {
		case <-testGCHookChan:
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
	afterCnt := int(gcTarget) + pinChunksCount
	runCountsTest(t, "after GC", db, afterCnt, afterCnt, 0, afterCnt, pinChunksCount, int(gcTarget))

	t.Run("pinned chunk not in gc Index", func(t *testing.T) {
		for _, addr := range pinAddrs {
			item := addressToItem(addr)
			i, err := db.retrievalAccessIndex.Get(item)
			if err != nil {
				t.Fatal(err)
			}
			item.AccessTimestamp = i.AccessTimestamp
			if _, err := db.gcIndex.Get(item); !errors.Is(err, leveldb.ErrNotFound) {
				t.Fatal("pinned chunk present in gcIndex")
			}
		}
	})

	t.Run("pinned chunks exist", func(t *testing.T) {
		if _, err := db.GetMulti(ctx, storage.ModeGetRequest, pinAddrs...); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("first chunks not pinned removed", func(t *testing.T) {
		recentIdx := chunkCount - int(dbCapacity) + int(gcTarget)
		_, err := db.GetMulti(ctx, storage.ModeGetRequest, addrs[recentIdx:]...)
		if err != nil {
			t.Fatal(err)
		}
	})
}

// Upload chunks, pin those chunks, add to GC after it is pinned
// check if the pinned files are still around
func TestGCAfterPin(t *testing.T) {
	ctx := context.Background()

	chunkCount := 50

	db := newTestDB(t, &Options{
		Capacity: 100,
	})

	pinAddrs := make([]swarm.Address, 0)

	// upload random chunks
	for i := 0; i < chunkCount; i++ {
		ch := generateTestRandomChunk()

		_, err := db.Put(ctx, storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}

		// Pin before adding to GC in ModeSetSync
		err = db.Set(ctx, storage.ModeSetPin, ch.Address())
		if err != nil {
			t.Fatal(err)
		}
		pinAddrs = append(pinAddrs, ch.Address())

		err = db.Set(ctx, storage.ModeSetSync, ch.Address())
		if err != nil {
			t.Fatal(err)
		}
	}

	t.Run("pin Index count", newItemsCountTest(db.pinIndex, chunkCount))

	t.Run("gc index count", newItemsCountTest(db.gcIndex, 0))

	for _, addr := range pinAddrs {
		_, err := db.Get(ctx, storage.ModeGetRequest, addr)
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestGCRequests
func TestGCRequests(t *testing.T) {
	ctx := context.Background()
	capacity := 100
	db := newTestDB(t, &Options{
		Capacity: uint64(capacity),
	})

	testGCHookChan := make(chan uint64)
	defer setTestHookCollectGarbage(func(collectedCount uint64) {
		testGCHookChan <- collectedCount
	})()

	// upload random chunks just up to the capacity
	chunks := generateTestRandomChunks(capacity)
	_, err := db.Put(ctx, storage.ModePutSync, chunks[1:]...)
	if err != nil {
		t.Fatal(err)
	}
	addrs := make([]swarm.Address, capacity)
	for i, ch := range chunks {
		addrs[i] = ch.Address()
	}

	// set update gc test hook to signal when
	// update gc goroutine is done by closing
	// testHookUpdateGCChan channel
	testHookUpdateGCChan := make(chan struct{})
	resetTestHookUpdateGC := setTestHookUpdateGC(func() {
		close(testHookUpdateGCChan)
	})

	// request the latest synced chunk
	// to prioritize it in the gc index
	// not to be collected
	_, err = db.GetMulti(ctx, storage.ModeGetRequest, addrs[1:10]...)
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
	_, err = db.Put(ctx, storage.ModePutSync, chunks[0])
	if err != nil {
		t.Fatal(err)
	}

	// wait for garbage collection
	gcTarget := db.gcTarget()
	var totalCollectedCount uint64
	for {
		select {
		case c := <-testGCHookChan:
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
	cnt := int(gcTarget)
	runCountsTest(t, "after GC", db, cnt, cnt, 0, cnt, 0, cnt)
	t.Run("requested chunks not removed", func(t *testing.T) {
		_, err := db.GetMulti(ctx, storage.ModeGetRequest, addrs[0:10]...)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("longest requested chunks get gc-ed", func(t *testing.T) {
		_, err := db.GetMulti(ctx, storage.ModeGetRequest, addrs[10:20]...)
		if !errors.Is(err, storage.ErrNotFound) {
			t.Errorf("got error %v, want %v", err, storage.ErrNotFound)
		}
	})

	// last synced chunk should not be removed
	t.Run("recent chunks not removed", func(t *testing.T) {
		_, err := db.GetMulti(ctx, storage.ModeGetRequest, addrs[20:]...)
		if err != nil {
			t.Fatal(err)
		}
	})
}

// TestDB_gcSize checks if gcSize has a correct value after
// database is initialized with existing data.
func TestDB_gcSize(t *testing.T) {
	ctx := context.Background()
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

		_, err := db.Put(ctx, storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}

		err = db.Set(ctx, storage.ModeSetSync, ch.Address())
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

// setTestHookCollectGarbage sets testGCHook and
// returns a function that will reset it to the
// value before the change.
func setTestHookCollectGarbage(h func(collectedCount uint64)) (reset func()) {
	current := testHookCollectGarbage
	reset = func() { testHookCollectGarbage = current }
	testHookCollectGarbage = h
	return reset
}

// TestSetTestGCHook tests if setTestHookCollectGarbage changes
// testGCHook function correctly and if its reset function
// resets the original function.
func TestSetTestGCHook(t *testing.T) {
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
	ctx := context.Background()
	db := newTestDB(t, &Options{
		Capacity: 10,
	})

	pinnedChunks := make([]swarm.Address, 0)

	// upload random chunks above db capacity to see if chunks are still pinned
	for i := 0; i < 20; i++ {
		ch := generateTestRandomChunk()
		_, err := db.Put(ctx, storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}
		err = db.Set(ctx, storage.ModeSetSync, ch.Address())
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
		_, err := db.Put(ctx, storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}
		err = db.Set(ctx, storage.ModeSetSync, ch.Address())
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 20; i++ {
		ch := generateTestRandomChunk()
		_, err := db.Put(ctx, storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}
		err = db.Set(ctx, storage.ModeSetSync, ch.Address())
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
		gotChunk, err := db.Get(ctx, storage.ModeGetRequest, swarm.NewAddress(outItem.Address))
		if err != nil {
			t.Fatal(err)
		}
		if !gotChunk.Address().Equal(swarm.NewAddress(addr.Bytes())) {
			t.Fatal("Pinned chunk is not equal to got chunk")
		}
	}

}

func generateAndPinAChunk(t *testing.T, db *DB) swarm.Chunk {
	ctx := context.Background()
	// Create a chunk and pin it
	pinnedChunk := generateTestRandomChunk()

	_, err := db.Put(ctx, storage.ModePutUpload, pinnedChunk)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Set(ctx, storage.ModeSetPin, pinnedChunk.Address())
	if err != nil {
		t.Fatal(err)
	}
	err = db.Set(ctx, storage.ModeSetSync, pinnedChunk.Address())
	if err != nil {
		t.Fatal(err)
	}
	return pinnedChunk
}

func TestPinSyncAndAccessPutSetChunkMultipleTimes(t *testing.T) {
	ctx := context.Background()
	var closed chan struct{}
	testGCHookChan := make(chan uint64)
	t.Cleanup(setTestHookCollectGarbage(func(collectedCount uint64) {
		select {
		case testGCHookChan <- collectedCount:
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
		_, err := db.Get(ctx, storage.ModeGetRequest, ch.Address())
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, ch := range rand1Chunks {
		_, err := db.Get(ctx, storage.ModeGetRequest, ch.Address())
		if err != nil {
			// ignore the chunks that are GCd
			continue
		}
	}

	rand2Chunks := addRandomChunks(t, 20, db, false)
	for _, ch := range rand2Chunks {
		_, err := db.Get(ctx, storage.ModeGetRequest, ch.Address())
		if err != nil {
			// ignore the chunks that are GCd
			continue
		}
	}

	rand3Chunks := addRandomChunks(t, 20, db, false)

	for _, ch := range rand3Chunks {
		_, err := db.Get(ctx, storage.ModeGetRequest, ch.Address())
		if err != nil {
			// ignore the chunks that are GCd
			continue
		}
	}

	// check if the pinned chunk is present after GC
	for _, ch := range pinnedChunks {
		gotChunk, err := db.Get(ctx, storage.ModeGetRequest, ch.Address())
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
	ctx := context.Background()
	var chunks []swarm.Chunk
	for i := 0; i < count; i++ {
		ch := generateTestRandomChunk()
		_, err := db.Put(ctx, storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}
		if pin {
			err = db.Set(ctx, storage.ModeSetPin, ch.Address())
			if err != nil {
				t.Fatal(err)
			}
			err = db.Set(ctx, storage.ModeSetSync, ch.Address())
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.Get(ctx, storage.ModeGetRequest, ch.Address())
			if err != nil {
				t.Fatal(err)
			}
		} else {
			// Non pinned chunks could be GC'd by the time they reach here.
			// so it is okay to ignore the error
			_ = db.Set(ctx, storage.ModeSetSync, ch.Address())
			_, _ = db.Get(ctx, storage.ModeGetRequest, ch.Address())
		}
		chunks = append(chunks, ch)
	}
	return chunks
}
