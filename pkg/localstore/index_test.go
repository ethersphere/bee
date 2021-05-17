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
	"math/rand"
	"testing"

	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestDB_pullIndex validates the ordering of keys in pull index.
// Pull index key contains PO prefix which is calculated from
// DB base key and chunk address. This is not an Item field
// which are checked in Mode tests.
// This test uploads chunks, sorts them in expected order and
// validates that pull index iterator will iterate it the same
// order.
func TestDB_pullIndex(t *testing.T) {
	db := newTestDB(t, nil)

	chunkCount := 50

	chunks := make([]testIndexChunk, chunkCount)

	// upload random chunks
	for i := 0; i < chunkCount; i++ {
		ch := generateTestRandomChunk()

		_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}

		chunks[i] = testIndexChunk{
			Chunk: ch,
			binID: uint64(i),
		}
	}

	testItemsOrder(t, db.pullIndex, chunks, func(i, j int) (less bool) {
		poi := swarm.Proximity(db.baseKey, chunks[i].Address().Bytes())
		poj := swarm.Proximity(db.baseKey, chunks[j].Address().Bytes())
		if poi < poj {
			return true
		}
		if poi > poj {
			return false
		}
		if chunks[i].binID < chunks[j].binID {
			return true
		}
		if chunks[i].binID > chunks[j].binID {
			return false
		}
		return bytes.Compare(chunks[i].Address().Bytes(), chunks[j].Address().Bytes()) == -1
	})
}

// TestDB_gcIndex validates garbage collection index by uploading
// a chunk with and performing operations using synced, access and
// request modes.
func TestDB_gcIndex(t *testing.T) {
	t.Cleanup(setWithinRadiusFunc(func(_ *DB, _ shed.Item) bool { return false }))
	db := newTestDB(t, nil)

	chunkCount := 50

	chunks := make([]testIndexChunk, chunkCount)

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

		chunks[i] = testIndexChunk{
			Chunk: ch,
		}
	}

	// check if all chunks are stored
	newItemsCountTest(db.pullIndex, chunkCount)(t)

	// check that chunks are not collectable for garbage
	newItemsCountTest(db.gcIndex, 0)(t)

	// set update gc test hook to signal when
	// update gc goroutine is done by sending to
	// testHookUpdateGCChan channel, which is
	// used to wait for indexes change verifications
	testHookUpdateGCChan := make(chan struct{})
	defer setTestHookUpdateGC(func() {
		testHookUpdateGCChan <- struct{}{}
	})()

	t.Run("request unsynced", func(t *testing.T) {
		ch := chunks[1]

		_, err := db.Get(context.Background(), storage.ModeGetRequest, ch.Address())
		if err != nil {
			t.Fatal(err)
		}
		// wait for update gc goroutine to be done
		<-testHookUpdateGCChan

		// the chunk is not synced
		// should not be in the garbace collection index
		newItemsCountTest(db.gcIndex, 0)(t)

		newIndexGCSizeTest(db)(t)
	})

	t.Run("sync one chunk", func(t *testing.T) {
		ch := chunks[0]

		err := db.Set(context.Background(), storage.ModeSetSync, ch.Address())
		if err != nil {
			t.Fatal(err)
		}

		// the chunk is synced and should be in gc index
		newItemsCountTest(db.gcIndex, 1)(t)

		newIndexGCSizeTest(db)(t)
	})

	t.Run("sync all chunks", func(t *testing.T) {
		for i := range chunks {
			err := db.Set(context.Background(), storage.ModeSetSync, chunks[i].Address())
			if err != nil {
				t.Fatal(err)
			}
		}

		testItemsOrder(t, db.gcIndex, chunks, nil)

		newIndexGCSizeTest(db)(t)
	})

	t.Run("request one chunk", func(t *testing.T) {
		i := 6

		_, err := db.Get(context.Background(), storage.ModeGetRequest, chunks[i].Address())
		if err != nil {
			t.Fatal(err)
		}
		// wait for update gc goroutine to be done
		<-testHookUpdateGCChan

		// move the chunk to the end of the expected gc
		c := chunks[i]
		chunks = append(chunks[:i], chunks[i+1:]...)
		chunks = append(chunks, c)

		testItemsOrder(t, db.gcIndex, chunks, nil)

		newIndexGCSizeTest(db)(t)
	})

	t.Run("random chunk request", func(t *testing.T) {

		rand.Shuffle(len(chunks), func(i, j int) {
			chunks[i], chunks[j] = chunks[j], chunks[i]
		})

		for _, ch := range chunks {
			_, err := db.Get(context.Background(), storage.ModeGetRequest, ch.Address())
			if err != nil {
				t.Fatal(err)
			}
			// wait for update gc goroutine to be done
			<-testHookUpdateGCChan
		}

		testItemsOrder(t, db.gcIndex, chunks, nil)

		newIndexGCSizeTest(db)(t)
	})

	t.Run("remove one chunk", func(t *testing.T) {
		i := 3

		err := db.Set(context.Background(), storage.ModeSetRemove, chunks[i].Address())
		if err != nil {
			t.Fatal(err)
		}

		// remove the chunk from the expected chunks in gc index
		chunks = append(chunks[:i], chunks[i+1:]...)

		testItemsOrder(t, db.gcIndex, chunks, nil)

		newIndexGCSizeTest(db)(t)
	})
}
