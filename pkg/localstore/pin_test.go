// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"context"
	"errors"
	"testing"

	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/storage"
)

func TestPinCounter(t *testing.T) {
	chunk := generateTestRandomChunk()
	db := newTestDB(t, nil)
	addr := chunk.Address()
	ctx := context.Background()
	_, err := db.Put(ctx, storage.ModePutUpload, chunk)
	if err != nil {
		t.Fatal(err)
	}
	var pinCounter uint64
	t.Run("+1 after first pin", func(t *testing.T) {
		err := db.Set(ctx, storage.ModeSetPin, addr)
		if err != nil {
			t.Fatal(err)
		}
		pinCounter, err = db.pinCounter(addr)
		if err != nil {
			t.Fatal(err)
		}
		if pinCounter != 1 {
			t.Fatalf("want pin counter %d but got %d", 1, pinCounter)
		}
	})
	t.Run("2 after second pin", func(t *testing.T) {
		err = db.Set(ctx, storage.ModeSetPin, addr)
		if err != nil {
			t.Fatal(err)
		}
		pinCounter, err = db.pinCounter(addr)
		if err != nil {
			t.Fatal(err)
		}
		if pinCounter != 2 {
			t.Fatalf("want pin counter %d but got %d", 2, pinCounter)
		}
	})
	t.Run("1 after first unpin", func(t *testing.T) {
		err = db.Set(ctx, storage.ModeSetUnpin, addr)
		if err != nil {
			t.Fatal(err)
		}
		pinCounter, err = db.pinCounter(addr)
		if err != nil {
			t.Fatal(err)
		}
		if pinCounter != 1 {
			t.Fatalf("want pin counter %d but got %d", 1, pinCounter)
		}
	})
	t.Run("not found after second unpin", func(t *testing.T) {
		err = db.Set(ctx, storage.ModeSetUnpin, addr)
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.pinCounter(addr)
		if !errors.Is(err, storage.ErrNotFound) {
			t.Fatal(err)
		}
	})
}

// Pin a file, upload chunks to go past the gc limit to trigger GC,
// check if the pinned files are still around and removed from gcIndex
func TestPinIndexes(t *testing.T) {
	ctx := context.Background()
	t.Cleanup(setWithinRadiusFunc(func(_ *DB, _ shed.Item) bool { return false }))

	db := newTestDB(t, &Options{
		Capacity: 150,
	})

	ch := generateTestRandomChunk()
	// call unreserve on the batch with radius 0 so that
	// localstore is aware of the batch and the chunk can
	// be inserted into the database
	unreserveChunkBatch(t, db, 0, ch)

	addr := ch.Address()
	_, err := db.Put(ctx, storage.ModePutUpload, ch)
	if err != nil {
		t.Fatal(err)
	}
	runCountsTest(t, "putUpload", db, 1, 0, 1, 1, 0, 0)

	err = db.Set(ctx, storage.ModeSetSync, addr)
	if err != nil {
		t.Fatal(err)
	}
	runCountsTest(t, "setSync", db, 1, 1, 0, 1, 0, 1)

	err = db.Set(ctx, storage.ModeSetPin, addr)
	if err != nil {
		t.Fatal(err)
	}
	runCountsTest(t, "setPin", db, 1, 1, 0, 1, 1, 0)

	err = db.Set(ctx, storage.ModeSetPin, addr)
	if err != nil {
		t.Fatal(err)
	}
	runCountsTest(t, "setPin 2", db, 1, 1, 0, 1, 1, 0)

	err = db.Set(ctx, storage.ModeSetUnpin, addr)
	if err != nil {
		t.Fatal(err)
	}
	runCountsTest(t, "setUnPin", db, 1, 1, 0, 1, 1, 0)

	err = db.Set(ctx, storage.ModeSetUnpin, addr)
	if err != nil {
		t.Fatal(err)
	}
	runCountsTest(t, "setUnPin 2", db, 1, 1, 0, 1, 0, 1)

}

func TestPinIndexesSync(t *testing.T) {
	ctx := context.Background()
	t.Cleanup(setWithinRadiusFunc(func(_ *DB, _ shed.Item) bool { return false }))

	db := newTestDB(t, &Options{
		Capacity: 150,
	})

	ch := generateTestRandomChunk()
	// call unreserve on the batch with radius 0 so that
	// localstore is aware of the batch and the chunk can
	// be inserted into the database
	unreserveChunkBatch(t, db, 0, ch)

	addr := ch.Address()
	_, err := db.Put(ctx, storage.ModePutUpload, ch)
	if err != nil {
		t.Fatal(err)
	}
	runCountsTest(t, "putUpload", db, 1, 0, 1, 1, 0, 0)

	err = db.Set(ctx, storage.ModeSetPin, addr)
	if err != nil {
		t.Fatal(err)
	}
	runCountsTest(t, "setPin", db, 1, 0, 1, 1, 1, 0)

	err = db.Set(ctx, storage.ModeSetPin, addr)
	if err != nil {
		t.Fatal(err)
	}
	runCountsTest(t, "setPin 2", db, 1, 0, 1, 1, 1, 0)

	err = db.Set(ctx, storage.ModeSetUnpin, addr)
	if err != nil {
		t.Fatal(err)
	}
	runCountsTest(t, "setUnPin", db, 1, 0, 1, 1, 1, 0)

	err = db.Set(ctx, storage.ModeSetUnpin, addr)
	if err != nil {
		t.Fatal(err)
	}
	runCountsTest(t, "setUnPin 2", db, 1, 0, 1, 1, 0, 0)

	err = db.Set(ctx, storage.ModeSetPin, addr)
	if err != nil {
		t.Fatal(err)
	}
	runCountsTest(t, "setPin 3", db, 1, 0, 1, 1, 1, 0)

	err = db.Set(ctx, storage.ModeSetSync, addr)
	if err != nil {
		t.Fatal(err)
	}
	runCountsTest(t, "setSync", db, 1, 1, 0, 1, 1, 0)

	err = db.Set(ctx, storage.ModeSetUnpin, addr)
	if err != nil {
		t.Fatal(err)
	}
	runCountsTest(t, "setUnPin", db, 1, 1, 0, 1, 0, 1)
}

func runCountsTest(t *testing.T, name string, db *DB, r, a, push, pull, pin, gc int) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		t.Helper()
		t.Run("retrieval data Index count", newItemsCountTest(db.retrievalDataIndex, r))
		t.Run("retrieval access Index count", newItemsCountTest(db.retrievalAccessIndex, a))
		t.Run("push Index count", newItemsCountTest(db.pushIndex, push))
		t.Run("pull Index count", newItemsCountTest(db.pullIndex, pull))
		t.Run("pin Index count", newItemsCountTest(db.pinIndex, pin))
		t.Run("gc index count", newItemsCountTest(db.gcIndex, gc))
		t.Run("gc size", newIndexGCSizeTest(db))
	})
}
