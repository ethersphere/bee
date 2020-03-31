// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package disk

import (
	"bytes"
	"context"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"io/ioutil"
	"os"
	"testing"
)

func TestMockStorer(t *testing.T) {

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
	if _, err := db.Get(ctx, keyFound); err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	if _, err := db.Get(ctx, keyNotFound); err != storage.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	chunk := swarm.NewChunk(keyFound, valueFound)
	if err := db.Put(ctx, chunk); err != nil {
		t.Fatalf("expected not error but got: %v", err)
	}

	if gotChunk, err := db.Get(ctx, keyFound); err != nil {
		t.Fatalf("expected not error but got: %v", err)

	} else {
		if !bytes.Equal(chunk.Data(), valueFound) {
			t.Fatalf("expected value %s but got %s", valueFound, gotChunk.Data())
		}
	}

	if yes, _ := db.Has(ctx, keyNotFound); yes {
		t.Fatalf("expected false but got true")
	}

	if yes, _ := db.Has(ctx, keyFound); !yes {
		t.Fatalf("expected true but got false")
	}
}


// newTestDB is a helper function that constructs a
// temporary database and returns a cleanup function that must
// be called to remove the data.
func newTestDB(t *testing.T) (db *diskStore, cleanupFunc func()) {
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
		db.Close()
		os.RemoveAll(dir)
	}
}