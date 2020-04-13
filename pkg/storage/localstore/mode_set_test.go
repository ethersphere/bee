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
	"context"
	"testing"
	"time"

	"github.com/ethersphere/swarm/chunk"
	tagtesting "github.com/ethersphere/swarm/chunk/testing"
	"github.com/ethersphere/swarm/shed"
	"github.com/syndtr/goleveldb/leveldb"
)

// TestModeSetAccess validates ModeSetAccess index values on the provided DB.
func TestModeSetAccess(t *testing.T) {
	for _, tc := range multiChunkTestCases {
		t.Run(tc.name, func(t *testing.T) {
			db, cleanupFunc := newTestDB(t, nil)
			defer cleanupFunc()

			chunks := generateTestRandomChunks(tc.count)

			wantTimestamp := time.Now().UTC().UnixNano()
			defer setNow(func() (t int64) {
				return wantTimestamp
			})()

			err := db.Set(context.Background(), chunk.ModeSetAccess, chunkAddresses(chunks)...)
			if err != nil {
				t.Fatal(err)
			}

			binIDs := make(map[uint8]uint64)

			for _, ch := range chunks {
				po := db.po(ch.Address())
				binIDs[po]++

				newPullIndexTest(db, ch, binIDs[po], nil)(t)
				newGCIndexTest(db, ch, wantTimestamp, wantTimestamp, binIDs[po], nil)(t)
			}

			t.Run("gc index count", newItemsCountTest(db.gcIndex, tc.count))

			t.Run("pull index count", newItemsCountTest(db.pullIndex, tc.count))

			t.Run("gc size", newIndexGCSizeTest(db))
		})
	}
}

// here we try to set a normal tag (that should be handled by pushsync)
// as a result we should expect the tag value to remain in the pull index
// and we expect that the tag should not be incremented by pull sync set
func TestModeSetSyncPullNormalTag(t *testing.T) {
	db, cleanupFunc := newTestDB(t, &Options{Tags: chunk.NewTags()})
	defer cleanupFunc()

	tag, err := db.tags.Create("test", 1, false)
	if err != nil {
		t.Fatal(err)
	}

	ch := generateTestRandomChunk().WithTagID(tag.Uid)
	_, err = db.Put(context.Background(), chunk.ModePutUpload, ch)
	if err != nil {
		t.Fatal(err)
	}

	tag.Inc(chunk.StateStored) // so we don't get an error on tag.Status later on

	item, err := db.pullIndex.Get(shed.Item{
		Address: ch.Address(),
		BinID:   1,
	})
	if err != nil {
		t.Fatal(err)
	}

	if item.Tag != tag.Uid {
		t.Fatalf("unexpected tag id value got %d want %d", item.Tag, tag.Uid)
	}

	err = db.Set(context.Background(), chunk.ModeSetSyncPull, ch.Address())
	if err != nil {
		t.Fatal(err)
	}

	item, err = db.pullIndex.Get(shed.Item{
		Address: ch.Address(),
		BinID:   1,
	})
	if err != nil {
		t.Fatal(err)
	}

	// expect the same tag Uid because when we set pull sync on a normal tag
	// the tag Uid should remain untouched in pull index
	if item.Tag != tag.Uid {
		t.Fatalf("unexpected tag id value got %d want %d", item.Tag, tag.Uid)
	}

	// 1 stored (because incremented manually in test), 1 sent, 1 total
	tagtesting.CheckTag(t, tag, 0, 1, 0, 1, 0, 1)
}

// TestModeSetSyncPullAnonymousTag checks that pull sync correcly increments
// counters on an anonymous tag which is expected to be handled only by pull sync
func TestModeSetSyncPullAnonymousTag(t *testing.T) {
	db, cleanupFunc := newTestDB(t, &Options{Tags: chunk.NewTags()})
	defer cleanupFunc()

	tag, err := db.tags.Create("test", 1, true)
	if err != nil {
		t.Fatal(err)
	}

	ch := generateTestRandomChunk().WithTagID(tag.Uid)
	_, err = db.Put(context.Background(), chunk.ModePutUpload, ch)
	if err != nil {
		t.Fatal(err)
	}
	tag.Inc(chunk.StateStored) // so we don't get an error on tag.Status later on

	item, err := db.pullIndex.Get(shed.Item{
		Address: ch.Address(),
		BinID:   1,
	})
	if err != nil {
		t.Fatal(err)
	}

	if item.Tag != tag.Uid {
		t.Fatalf("unexpected tag id value got %d want %d", item.Tag, tag.Uid)
	}

	err = db.Set(context.Background(), chunk.ModeSetSyncPull, ch.Address())
	if err != nil {
		t.Fatal(err)
	}

	item, err = db.pullIndex.Get(shed.Item{
		Address: ch.Address(),
		BinID:   1,
	})
	if err != nil {
		t.Fatal(err)
	}

	if item.Tag != 0 {
		t.Fatalf("unexpected tag id value got %d want %d", item.Tag, 0)
	}

	// 1 stored (because incremented manually in test), 1 sent, 1 total
	tagtesting.CheckTag(t, tag, 0, 1, 0, 1, 0, 1)
}

// TestModeSetSyncPullPushAnonymousTag creates an anonymous tag and a corresponding chunk
// then tries to Set both with push and pull Sync modes, but asserts that only the pull sync
// increments were done to the tag
func TestModeSetSyncPullPushAnonymousTag(t *testing.T) {
	db, cleanupFunc := newTestDB(t, &Options{Tags: chunk.NewTags()})
	defer cleanupFunc()

	tag, err := db.tags.Create("test", 1, true)
	if err != nil {
		t.Fatal(err)
	}

	ch := generateTestRandomChunk().WithTagID(tag.Uid)
	_, err = db.Put(context.Background(), chunk.ModePutUpload, ch)
	if err != nil {
		t.Fatal(err)
	}

	tag.Inc(chunk.StateStored) // so we don't get an error on tag.Status later on
	item, err := db.pullIndex.Get(shed.Item{
		Address: ch.Address(),
		BinID:   1,
	})
	if err != nil {
		t.Fatal(err)
	}

	if item.Tag != tag.Uid {
		t.Fatalf("unexpected tag id value got %d want %d", item.Tag, tag.Uid)
	}

	err = db.Set(context.Background(), chunk.ModeSetSyncPull, ch.Address())
	if err != nil {
		t.Fatal(err)
	}

	// expect no error here. if the item cannot be found in pushsync the rest of the
	// setSync logic should be executed
	err = db.Set(context.Background(), chunk.ModeSetSyncPush, ch.Address())
	if err != nil {
		t.Fatal(err)
	}

	// check that the tag has been incremented
	item, err = db.pullIndex.Get(shed.Item{
		Address: ch.Address(),
		BinID:   1,
	})
	if err != nil {
		t.Fatal(err)
	}

	if item.Tag != 0 {
		t.Fatalf("unexpected tag id value got %d want %d", item.Tag, 0)
	}

	// 1 stored (because incremented manually in test), 1 sent, 1 total
	tagtesting.CheckTag(t, tag, 0, 1, 0, 1, 0, 1)

	// verify that the item does not exist in the push index
	item, err = db.pushIndex.Get(shed.Item{
		Address: ch.Address(),
		BinID:   1,
	})
	if err == nil {
		t.Fatal("expected error but got none")
	}
}

// TestModeSetSyncPushNormalTag makes sure that push sync increments tags
// correctly on a normal tag (that is, a tag that is expected to show progress bars
// according to push sync progress)
func TestModeSetSyncPushNormalTag(t *testing.T) {
	db, cleanupFunc := newTestDB(t, &Options{Tags: chunk.NewTags()})
	defer cleanupFunc()

	tag, err := db.tags.Create("test", 1, false)
	if err != nil {
		t.Fatal(err)
	}

	ch := generateTestRandomChunk().WithTagID(tag.Uid)
	_, err = db.Put(context.Background(), chunk.ModePutUpload, ch)
	if err != nil {
		t.Fatal(err)
	}

	tag.Inc(chunk.StateStored) // so we don't get an error on tag.Status later on
	item, err := db.pullIndex.Get(shed.Item{
		Address: ch.Address(),
		BinID:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Tag != tag.Uid {
		t.Fatalf("unexpected tag id value got %d want %d", item.Tag, tag.Uid)
	}

	tagtesting.CheckTag(t, tag, 0, 1, 0, 0, 0, 1)

	err = db.Set(context.Background(), chunk.ModeSetSyncPush, ch.Address())
	if err != nil {
		t.Fatal(err)
	}

	item, err = db.pullIndex.Get(shed.Item{
		Address: ch.Address(),
		BinID:   1,
	})
	if err != nil {
		t.Fatal(err)
	}

	if item.Tag != tag.Uid {
		t.Fatalf("unexpected tag id value got %d want %d", item.Tag, tag.Uid)
	}

	tagtesting.CheckTag(t, tag, 0, 1, 0, 0, 1, 1)

	// call pull sync set, expect no changes
	err = db.Set(context.Background(), chunk.ModeSetSyncPull, ch.Address())
	if err != nil {
		t.Fatal(err)
	}

	item, err = db.pullIndex.Get(shed.Item{
		Address: ch.Address(),
		BinID:   1,
	})
	if err != nil {
		t.Fatal(err)
	}

	tagtesting.CheckTag(t, tag, 0, 1, 0, 0, 1, 1)

	if item.Tag != tag.Uid {
		t.Fatalf("unexpected tag id value got %d want %d", item.Tag, tag.Uid)
	}
}

// TestModeSetRemove validates ModeSetRemove index values on the provided DB.
func TestModeSetRemove(t *testing.T) {
	for _, tc := range multiChunkTestCases {
		t.Run(tc.name, func(t *testing.T) {
			db, cleanupFunc := newTestDB(t, nil)
			defer cleanupFunc()

			chunks := generateTestRandomChunks(tc.count)

			_, err := db.Put(context.Background(), chunk.ModePutUpload, chunks...)
			if err != nil {
				t.Fatal(err)
			}

			err = db.Set(context.Background(), chunk.ModeSetRemove, chunkAddresses(chunks)...)
			if err != nil {
				t.Fatal(err)
			}

			t.Run("retrieve indexes", func(t *testing.T) {
				for _, ch := range chunks {
					wantErr := leveldb.ErrNotFound
					_, err := db.retrievalDataIndex.Get(addressToItem(ch.Address()))
					if err != wantErr {
						t.Errorf("got error %v, want %v", err, wantErr)
					}

					// access index should not be set
					_, err = db.retrievalAccessIndex.Get(addressToItem(ch.Address()))
					if err != wantErr {
						t.Errorf("got error %v, want %v", err, wantErr)
					}
				}

				t.Run("retrieve data index count", newItemsCountTest(db.retrievalDataIndex, 0))

				t.Run("retrieve access index count", newItemsCountTest(db.retrievalAccessIndex, 0))
			})

			for _, ch := range chunks {
				newPullIndexTest(db, ch, 0, leveldb.ErrNotFound)(t)
			}

			t.Run("pull index count", newItemsCountTest(db.pullIndex, 0))

			t.Run("gc index count", newItemsCountTest(db.gcIndex, 0))

			t.Run("gc size", newIndexGCSizeTest(db))
		})
	}
}
