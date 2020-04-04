// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package mem_test

import (
	"bytes"
	"context"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethersphere/bee/pkg/swarm"
	"math/rand"
	"testing"

	"github.com/ethersphere/bee/pkg/storage"
	mem "github.com/ethersphere/bee/pkg/storage/mem"
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

var allKeys = []string{
	"0xaaaac30623a1a20c48c473e55c8456944772b477",
	"0xabbbc30623a1a20c48c473e55c8456944772b477",
	"0xabccc30623a1a20c48c473e55c8456944772b477",
	"0xdaaac30623a1a20c48c473e55c8456944772b477",
	"0xdbaac30623a1a20c48c473e55c8456944772b477",
	"0xdbbac30623a1a20c48c473e55c8456944772b477",
	"0xeeeee30623a1a20c48c473e55c8456944772b477",
	"0xfffff30623a1a20c48c473e55c8456944772b477",
}

func addItemsToDB(t *testing.T, ctx context.Context, db *mem.MemStore) {
	t.Helper()
	for _, k := range allKeys {
		v := allItems[k]
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

// newTestDB is a helper function that constructs a
// temporary database and returns a cleanup function that must
// be called to remove the data.
func newTestDB(t *testing.T) (db *mem.MemStore, cleanupFunc func()) {
	t.Helper()

	db, err := mem.NewMemStorer(storage.ValidateContentChunk)
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
	ch := GenerateRandomChunk()

	t.Run("put", func(t *testing.T) {

		if _, err := db.Get(ctx, ch.Address().Bytes()); err != storage.ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}

		if err := db.Put(ctx, ch.Address().Bytes(), ch.Data().Bytes()); err != nil {
			t.Fatalf("expected not error but got: %v", err)
		}

		gotVal, err := db.Get(ctx, ch.Address().Bytes())
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}

		if !bytes.Equal(ch.Data().Bytes(), gotVal) {
			t.Fatalf("expected %v, got %v", allItems["aaaa"], string(gotVal))
		}

	})

	t.Run("get", func(t *testing.T) {
		if gotValue, err := db.Get(ctx, ch.Address().Bytes()); err != nil {
			t.Fatalf("expected not error but got: %v", err)

		} else {
			if !bytes.Equal(gotValue, ch.Data().Bytes()) {
				t.Fatalf("expected value len %d but got len %d", len(ch.Data().Bytes()), len(gotValue))
			}
		}
	})

	t.Run("has", func(t *testing.T) {
		// Check if a non existing key is found or not.
		if yes, _ := db.Has(ctx, []byte("NonexistantKey")); yes {
			t.Fatalf("expected false but got true")
		}

		// Check if an existing key is found.
		if yes, _ := db.Has(ctx, ch.Address().Bytes()); !yes {
			t.Fatalf("expected true but got false")
		}
	})

	t.Run("delete", func(t *testing.T) {
		// Try deleting a non existing key.
		if err := db.Delete(ctx, []byte("NonexistantKey")); err != nil {
			t.Fatalf("expected no error but got: %v", err)
		}

		// Delete a existing key.
		if err := db.Delete(ctx, ch.Address().Bytes()); err != nil {
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

func GenerateRandomChunk() swarm.Chunk {
	data := make([]byte, swarm.DefaultChunkSize)
	rand.Read(data)
	key := make([]byte, swarm.DefaultAddressLength)
	rand.Read(key)
	return swarm.NewChunk(swarm.NewAddress(key), swarm.NewData(data))
}
