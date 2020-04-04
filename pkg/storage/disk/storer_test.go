// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package disk

import (
	"bytes"
	"context"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"io/ioutil"
	"os"
	"testing"

	"github.com/ethersphere/bee/pkg/storage"
)

var allItems = map[string]string{
	"0xaaaac30623a1a20c48c473e55c8456944772b477": "data80",
	"0xabbbc30623a1a20c48c473e55c8456944772b477": "data81",
	"0xabccc30623a1a20c48c473e55c8456944772b477": "data82",
	"0xdaaac30623a1a20c48c473e55c8456944772b477": "data83",
	"0xdbaac30623a1a20c48c473e55c8456944772b477": "data84",
	"0xdbbac30623a1a20c48c473e55c8456944772b477": "data85",
	"0xeeeee30623a1a20c48c473e55c8456944772b477": "data90",
	"0xfffff30623a1a20c48c473e55c8456944772b477": "data91",
}

func addItemsToDB(t *testing.T, ctx context.Context, db *DiskStore) {
	t.Helper()
	for k, v := range allItems {
		hexBytes, err := hexutil.Decode(k)
		if err != nil {
			t.Fatalf("%v", err)
		}
		err = db.Put(ctx, hexBytes, []byte(v))
		if err != nil {
			t.Fatalf("%v", err)
		}
	}
}

// TestDiskStorerGetHasDelete tests Get , Put, Has and Delete functions of the DB.
func TestDiskStorerGetPutHasDelete(t *testing.T) {
	db, clean := newTestDB(t)
	defer clean()
	ctx := context.Background()
	hexBytes, err := hexutil.Decode("0xaaaac30623a1a20c48c473e55c8456944772b477")
	if err != nil {
		t.Fatalf("%v", err)
	}

	t.Run("put", func(t *testing.T) {
		if _, err := db.Get(ctx, []byte("0xaaaac30623a1a20c48c473e55c8456944772b477")); err != storage.ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}

		if err := db.Put(ctx, hexBytes, []byte(allItems["0xaaaac30623a1a20c48c473e55c8456944772b477"])); err != nil {
			t.Fatalf("expected not error but got: %v", err)
		}

		gotVal, err := db.Get(ctx, hexBytes)
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}

		if !bytes.Equal([]byte(allItems["0xaaaac30623a1a20c48c473e55c8456944772b477"]), gotVal) {
			t.Fatalf("expected %v, got %v", allItems["0xaaaac30623a1a20c48c473e55c8456944772b477"], string(gotVal))
		}

	})

	t.Run("get", func(t *testing.T) {
		if gotValue, err := db.Get(ctx, hexBytes); err != nil {
			t.Fatalf("expected not error but got: %v", err)

		} else {
			if !bytes.Equal(gotValue, []byte(allItems["0xaaaac30623a1a20c48c473e55c8456944772b477"])) {
				t.Fatalf("expected value %s but got %s", allItems["0xaaaac30623a1a20c48c473e55c8456944772b477"], string(gotValue))
			}
		}
	})

	t.Run("has", func(t *testing.T) {
		// Check if a non existing key is found or not.
		if yes, _ := db.Has(ctx, []byte("NonexistantKey")); yes {
			t.Fatalf("expected false but got true")
		}

		// Check if an existing key is found.
		if yes, _ := db.Has(ctx, hexBytes); !yes {
			t.Fatalf("expected true but got false")
		}
	})

	t.Run("delete", func(t *testing.T) {
		// Try deleting a non existing key.
		if err := db.Delete(ctx, []byte("NonexistantKey")); err != nil {
			t.Fatalf("expected no error but got: %v", err)
		}

		// Delete a existing key.
		if err := db.Delete(ctx, hexBytes); err != nil {
			t.Fatalf("expected no error but got: %v", err)
		}
	})
}

// newTestDB is a helper function that constructs a
// temporary database and returns a cleanup function that must
// be called to remove the data.
func newTestDB(t *testing.T) (db *DiskStore, cleanupFunc func()) {
	t.Helper()

	dir, err := ioutil.TempDir("", "disk-test")
	if err != nil {
		t.Fatal(err)
	}

	db, err = NewDiskStorer(dir, storage.ValidateContentChunk)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatal(err)
	}
	return db, func() {
		db.Close(context.Background())
		os.RemoveAll(dir)
	}
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
		hexBytes, err := hexutil.Decode("0xdb")
		if err != nil {
			t.Fatalf("%v", err)
		}
		count, err := db.CountPrefix(hexBytes)
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
		hexBytes, err := hexutil.Decode("0xdb")
		if err != nil {
			t.Fatalf("%v", err)
		}
		count, err := db.CountFrom(hexBytes)
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
			data, ok := allItems[hexutil.Encode(key)]
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
		hexBytes, err := hexutil.Decode("0xdb")
		if err != nil {
			t.Fatalf("%v", err)
		}
		err = db.Iterate(hexBytes, false, func(key []byte, value []byte) (stop bool, err error) {
			data, ok := allItems[hexutil.Encode(key)]
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

		if count != 4 {
			t.Fatalf("iterated %v expected %v", count, 5)
		}
	})

	t.Run("iterateFromPrefixSkipStartKey", func(t *testing.T) {
		var count int
		// check for iteration correctness.
		hexBytes, err := hexutil.Decode("0xab")
		if err != nil {
			t.Fatalf("%v", err)
		}
		err = db.Iterate(hexBytes, true, func(key []byte, value []byte) (stop bool, err error) {
			data, ok := allItems[hexutil.Encode(key)]
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
		if !bytes.Equal([]byte(allItems["0xaaaac30623a1a20c48c473e55c8456944772b477"]), v) {
			t.Fatalf("expected %v got %v", allItems["0xaaaac30623a1a20c48c473e55c8456944772b477"], string(v))
		}
	})

	t.Run("firstWithPrefix", func(t *testing.T) {
		hexBytes, err := hexutil.Decode("0xda")
		if err != nil {
			t.Fatalf("%v", err)
		}
		_, v, err := db.First(hexBytes)
		if err != nil {
			t.Fatalf("%v", err)
		}
		if !bytes.Equal([]byte(allItems["0xdaaac30623a1a20c48c473e55c8456944772b477"]), v) {
			t.Fatalf("expected %v got %v", allItems["0xdaaac30623a1a20c48c473e55c8456944772b477"], string(v))
		}
	})

	t.Run("last", func(t *testing.T) {
		_, v, err := db.Last(nil)
		if err != nil {
			t.Fatalf("%v", err)
		}
		if !bytes.Equal([]byte(allItems["0xfffff30623a1a20c48c473e55c8456944772b477"]), v) {
			t.Fatalf("expected %v got %v", allItems["0xfffff30623a1a20c48c473e55c8456944772b477"], string(v))
		}
	})

	t.Run("lastWithPrefix", func(t *testing.T) {
		hexBytes, err := hexutil.Decode("0xdb")
		if err != nil {
			t.Fatalf("%v", err)
		}
		_, v, err := db.Last(hexBytes)
		if err != nil {
			t.Fatalf("%v", err)
		}
		if !bytes.Equal([]byte(allItems["0xdbbac30623a1a20c48c473e55c8456944772b477"]), v) {
			t.Fatalf("expected %v got %v", allItems["0xdbbac30623a1a20c48c473e55c8456944772b477"], string(v))
		}
	})
}

func TestBatch(t *testing.T) {
	db, clean := newTestDB(t)
	defer clean()

	ctx := context.Background()
	addItemsToDB(t, ctx, db)

	t.Run("readWriteBatch", func(t *testing.T) {
		batch := db.GetBatch(false)

		err := batch.Set([]byte("0xeeeee30623a1a20c48c473e55c8456944772b477"), []byte("XXXXc30623a1a20c48c473e55c8456944772b477"))
		if err != nil {
			t.Fatalf("%v", err)
		}

		// Check if the value is not reflected before the WriteBatch
		_, err = db.Get(ctx, []byte("0xeeeee30623a1a20c48c473e55c8456944772b477"))
		if err != storage.ErrNotFound {
			t.Fatalf("%v", err)
		}

		err = db.WriteBatch(batch)
		if err != nil {
			t.Fatalf("%v", err)
		}

		// Check if the values reflected after the WriteBatch
		value, err := db.Get(ctx, []byte("0xeeeee30623a1a20c48c473e55c8456944772b477"))
		if err != nil {
			t.Fatalf("%v", err)
		}
		if !bytes.Equal([]byte("XXXXc30623a1a20c48c473e55c8456944772b477"), value) {
			t.Fatalf("expected %v got %v", "XXXX", string(value))
		}
	})
}

func TestPersistenceAfterDBClose(t *testing.T) {
	dir, err := ioutil.TempDir("", "disk-test")
	if err != nil {
		t.Fatal(err)
	}

	// Open a new DB.
	db, err := NewDiskStorer(dir, storage.ValidateContentChunk)
	if err != nil {
		err = os.RemoveAll(dir)
		if err != nil {
			t.Fatal(err)
		}
		t.Fatal(err)
	}

	// Add some items to the DB.
	ctx := context.Background()
	addItemsToDB(t, ctx, db)

	// close the DB.
	err = db.Close(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Open the DB again.
	db, err = NewDiskStorer(dir, storage.ValidateContentChunk)
	if err != nil {
		err = os.RemoveAll(dir)
		if err != nil {
			t.Fatal(err)
		}
		t.Fatal(err)
	}

	// Check if all the items are still intact.
	for k, v := range allItems {
		hexBytes, err := hexutil.Decode(k)
		if err != nil {
			t.Fatalf("%v", err)
		}
		gotVal, err := db.Get(ctx, hexBytes)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal([]byte(v), gotVal) {
			t.Fatalf("expected %v got %v", k, string(gotVal))
		}
	}

	// close and Remove the DB.
	err = db.Close(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = os.RemoveAll(dir)
	if err != nil {
		t.Fatal(err)
	}

}
