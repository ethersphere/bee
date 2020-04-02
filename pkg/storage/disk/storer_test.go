// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package disk

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io/ioutil"
	"os"
	"testing"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestDiskStorer(t *testing.T) {
	db, clean := newTestDB(t)
	defer clean()

	keyFound, err := swarm.ParseHexAddress("aabbcc")
	if err != nil {
		t.Fatal(err)
	}
	keyNotFound, err := swarm.ParseHexAddress("bbccdd")
	if err != nil {
		t.Fatal(err)
	}

	valueFound := []byte("data data data")

	ctx := context.Background()
	if _, err := db.Get(ctx, keyFound.Bytes()); err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	if _, err := db.Get(ctx, keyNotFound.Bytes()); err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	if err := db.Put(ctx, keyFound.Bytes(), valueFound); err != nil {
		t.Fatalf("expected not error but got: %v", err)
	}

	if gotValue, err := db.Get(ctx, keyFound.Bytes()); err != nil {
		t.Fatalf("expected not error but got: %v", err)

	} else {
		if !bytes.Equal(gotValue, valueFound) {
			t.Fatalf("expected value %s but got %s", valueFound, gotValue)
		}
	}

	// Check if a non existing key is found or not.
	if yes, _ := db.Has(ctx, keyNotFound.Bytes()); yes {
		t.Fatalf("expected false but got true")
	}

	// Check if an existing key is found.
	if yes, _ := db.Has(ctx, keyFound.Bytes()); !yes {
		t.Fatalf("expected true but got false")
	}

	// Try deleting a non existing key.
	if err := db.Delete(ctx, keyNotFound.Bytes()); err != nil {
		t.Fatalf("expected no error but got: %v", err)
	}

	// Delete a existing key.
	if err := db.Delete(ctx, keyFound.Bytes()); err != nil {
		t.Fatalf("expected no error but got: %v", err)
	}
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
	db, err = NewDiskStorer(dir)
	if err != nil {
		os.RemoveAll(dir)
		t.Fatal(err)
	}
	return db, func() {
		db.Close(context.Background())
		os.RemoveAll(dir)
	}
}

func TestCountAndIterator(t *testing.T) {
	db, clean := newTestDB(t)
	defer clean()

	expectedMap := make(map[string][]byte)

	ctx := context.Background()
	for i := 0; i < 100; i++ {
		ch := GenerateRandomChunk()
		expectedMap[ch.Address().String()] = ch.Data().Bytes()
		err := db.Put(ctx, ch.Address().Bytes(), ch.Data().Bytes())
		if err != nil {
			t.Fatalf("%v", err)
		}
	}

	count, err := db.Count(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}

	// check count match.
	if count != 100 {
		t.Fatalf("expected %v but got: %v", 100, count)
	}

	// check for iteration correctness.
	err = db.Iterate([]byte{0}, false, func(key []byte, value []byte) (stop bool, err error) {
		data, ok := expectedMap[hex.EncodeToString(key)]
		if !ok {
			t.Fatalf("key %v not present", hex.EncodeToString(key))
		}

		if !bytes.Equal(data, value) {
			t.Fatalf("data mismatch for key %v", hex.EncodeToString(key))
			return true, nil
		}
		return false, nil
	})

	if err != nil {
		t.Fatalf("error in iteration")
	}
}

func GenerateRandomChunk() swarm.Chunk {
	data := make([]byte, swarm.DefaultChunkSize)
	rand.Read(data)
	key := make([]byte, swarm.DefaultAddressLength)
	rand.Read(key)
	return swarm.NewChunk(swarm.NewAddress(key), swarm.NewData(data))
}
