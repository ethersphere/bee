// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mem_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/ethersphere/bee/pkg/storage"
	mem "github.com/ethersphere/bee/pkg/storage/mem"
)

var allItems = map[string]string{
	"aaaa": "data80",
	"abbb": "data81",
	"abcc": "data82",
	"daaa": "data83",
	"dbaa": "data84",
	"dbba": "data85",
	"xxxx": "data90",
	"zzzz": "data91",
}

var allKeys = []string{
	"aaaa",
	"abbb",
	"abcc",
	"daaa",
	"dbaa",
	"dbba",
	"xxxx",
	"zzzz",
}

func addItemsToDB(t *testing.T, ctx context.Context, db *mem.MemStore) {
	t.Helper()
	for _, k := range allKeys {
		v := allItems[string(k)]
		err := db.Put(ctx, []byte(k), []byte(v))
		if err != nil {
			t.Fatalf("%v", err)
		}
	}
}

// newTestDB is a helper function that constructs a
// temporary database and returns a cleanup function that must
// be called to remove the data.
func newTestDB(t *testing.T) (db *mem.MemStore, cleanupFunc func()) {
	t.Helper()
	db, err := mem.NewMemStorer()
	if err != nil {
		t.Fatal(err)
	}
	return db, func() {
		db.Close(context.Background())
	}
}

// TestDiskStorerGetHasDelete tests Get , Put, Has and Delete functions of the DB.
func TestDiskStorerGetPutHasDelete(t *testing.T) {
	db, clean := newTestDB(t)
	defer clean()
	ctx := context.Background()

	t.Run("put", func(t *testing.T) {
		if _, err := db.Get(ctx, []byte("aaaa")); err != storage.ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}

		if err := db.Put(ctx, []byte("aaaa"), []byte(allItems["aaaa"])); err != nil {
			t.Fatalf("expected not error but got: %v", err)
		}

		gotVal, err := db.Get(ctx, []byte("aaaa"))
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}

		if !bytes.Equal([]byte(allItems["aaaa"]), gotVal) {
			t.Fatalf("expected %v, got %v", allItems["aaaa"], string(gotVal))
		}

	})

	t.Run("get", func(t *testing.T) {
		if gotValue, err := db.Get(ctx, []byte("aaaa")); err != nil {
			t.Fatalf("expected not error but got: %v", err)

		} else {
			if !bytes.Equal(gotValue, []byte(allItems["aaaa"])) {
				t.Fatalf("expected value %s but got %s", allItems["aaaa"], string(gotValue))
			}
		}
	})

	t.Run("has", func(t *testing.T) {
		// Check if a non existing key is found or not.
		if yes, _ := db.Has(ctx, []byte("NonexistantKey")); yes {
			t.Fatalf("expected false but got true")
		}

		// Check if an existing key is found.
		if yes, _ := db.Has(ctx, []byte("aaaa")); !yes {
			t.Fatalf("expected true but got false")
		}
	})

	t.Run("delete", func(t *testing.T) {
		// Try deleting a non existing key.
		if err := db.Delete(ctx, []byte("NonexistantKey")); err != nil {
			t.Fatalf("expected no error but got: %v", err)
		}

		// Delete a existing key.
		if err := db.Delete(ctx, []byte("aaaa")); err != nil {
			t.Fatalf("expected no error but got: %v", err)
		}
	})
}

func TestCount(t *testing.T) {
	db, clean := newTestDB(t)
	defer clean()

	ctx := context.Background()
	addItemsToDB(t, ctx, db)

	t.Run("count", func(t *testing.T) {
		// Check the total count
		count, err := db.Count(ctx)
		if err != nil {
			t.Fatalf("%v", err)
		}
		// check count match.
		if count != len(allItems) {
			t.Fatalf("expected %v but got: %v", len(allItems), count)
		}
	})

	t.Run("countPrefix", func(t *testing.T) {
		// Check CountPrefix.
		count, err := db.CountPrefix([]byte("db"))
		if err != nil {
			t.Fatalf("%v", err)
		}
		if count != 2 {
			t.Fatalf("expected %v but got: %v", 2, count)
		}
	})

	t.Run("countPrefixFromBegining", func(t *testing.T) {
		// Check CountPrefix.
		count, err := db.CountPrefix(nil)
		if err != nil {
			t.Fatalf("%v", err)
		}
		if count != len(allItems) {
			t.Fatalf("expected %v but got: %v", len(allItems), count)
		}
	})

	t.Run("countFrom", func(t *testing.T) {
		// Check the CountFrom.
		count, err := db.CountFrom([]byte("db"))
		if err != nil {
			t.Fatalf("%v", err)
		}
		if count != 4 {
			t.Fatalf("expected %v but got: %v", 4, count)
		}
	})

}

func TestIterator(t *testing.T) {
	db, clean := newTestDB(t)
	defer clean()

	ctx := context.Background()
	addItemsToDB(t, ctx, db)

	t.Run("iterateAll", func(t *testing.T) {
		var count int
		// check for iteration correctness.
		err := db.Iterate([]byte{0}, false, func(key []byte, value []byte) (stop bool, err error) {
			data, ok := allItems[string(key)]
			if !ok {
				t.Fatalf("key %v not present", string(key))
			}

			if !bytes.Equal([]byte(data), value) {
				t.Fatalf("data mismatch for key %v", string(key))
				return true, nil
			}
			count++
			return false, nil
		})

		if err != nil {
			t.Fatalf("error in iteration")
		}

		if count != len(allItems) {
			t.Fatalf("iterated %v expected %v", count, len(allItems))
		}
	})

	t.Run("iterateFromPrefix", func(t *testing.T) {
		var count int
		// check for iteration correctness.
		err := db.Iterate([]byte("d"), false, func(key []byte, value []byte) (stop bool, err error) {
			data, ok := allItems[string(key)]
			if !ok {
				t.Fatalf("key %v not present", string(key))
			}

			if !bytes.Equal([]byte(data), value) {
				t.Fatalf("data mismatch for key %v", string(key))
				return true, nil
			}
			count++
			return false, nil
		})

		if err != nil {
			t.Fatalf("error in iteration")
		}

		if count != 5 {
			t.Fatalf("iterated %v expected %v", count, 5)
		}
	})

	t.Run("iterateFromPrefixSkipStartKey", func(t *testing.T) {
		var count int
		// check for iteration correctness.
		err := db.Iterate([]byte("ab"), true, func(key []byte, value []byte) (stop bool, err error) {
			data, ok := allItems[string(key)]
			if !ok {
				t.Fatalf("key %v not present", string(key))
			}

			if !bytes.Equal([]byte(data), value) {
				t.Fatalf("data mismatch for key %v", string(key))
				return true, nil
			}
			count++
			return false, nil
		})

		if err != nil {
			t.Fatalf("error in iteration")
		}

		if count != 6 {
			t.Fatalf("iterated %v expected %v", count, 6)
		}
	})
}

func TestFirstAndLast(t *testing.T) {
	db, clean := newTestDB(t)
	defer clean()

	ctx := context.Background()
	addItemsToDB(t, ctx, db)

	t.Run("first", func(t *testing.T) {
		_, v, err := db.First(nil)
		if err != nil {
			t.Fatalf("%v", err)
		}
		if !bytes.Equal([]byte(allItems["aaaa"]), v) {
			t.Fatalf("expected %v got %v", allItems["aaaa"], string(v))
		}
	})

	t.Run("firstWithPrefix", func(t *testing.T) {
		_, v, err := db.First([]byte("da"))
		if err != nil {
			t.Fatalf("%v", err)
		}
		if !bytes.Equal([]byte(allItems["daaa"]), v) {
			t.Fatalf("expected %v got %v", allItems["daaa"], string(v))
		}
	})

	t.Run("last", func(t *testing.T) {
		_, v, err := db.Last(nil)
		if err != nil {
			t.Fatalf("%v", err)
		}
		if !bytes.Equal([]byte(allItems["zzzz"]), v) {
			t.Fatalf("expected %v got %v", allItems["zzzz"], string(v))
		}
	})

	t.Run("lastWithPrefix", func(t *testing.T) {
		_, v, err := db.Last([]byte("db"))
		if err != nil {
			t.Fatalf("%v", err)
		}
		if !bytes.Equal([]byte(allItems["dbba"]), v) {
			t.Fatalf("expected %v got %v", allItems["dbba"], string(v))
		}
	})
}

func TestBatch(t *testing.T) {
	db, clean := newTestDB(t)
	defer clean()

	ctx := context.Background()
	addItemsToDB(t, ctx, db)

	t.Run("readWriteBatch", func(t *testing.T) {
		batch := db.GetBatch(true)
		if batch != nil {
			t.Fatalf("expected %v, got %v", storage.ErrNotImplemented, batch)
		}

		err := db.WriteBatch(batch)
		if err != storage.ErrNotImplemented {
			t.Fatalf("expected %v, got %v", storage.ErrNotImplemented, err)
		}
	})
}
