// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"context"
	"errors"
	"testing"

	"github.com/ethersphere/bee/pkg/storagev2"
	"github.com/google/go-cmp/cmp"
)

// BatchedStore is a store that provides batch operations.
type BatchedStore interface {
	storage.Store
	storage.Batcher
}

func TestBatchedStore(t *testing.T, bs BatchedStore) {
	item := &obj1{Id: "id", SomeInt: 1, Buf: []byte("data")}

	t.Run("duplicates are rejected", func(t *testing.T) {
		batch, err := bs.Batch(context.Background())
		if err != nil {
			t.Fatalf("Batch(...): unexpected error: %v", err)
		}

		if err := batch.Put(item); err != nil {
			t.Fatalf("Put(...): unexpected error: %v", err)
		}

		if err := batch.Put(item); err != nil {
			t.Fatalf("Put(...): unexpected error: %v", err)
		}
		if err := batch.Commit(); err != nil {
			t.Fatalf("Commit(): unexpected error: %v", err)
		}

		var cnt int
		err = bs.Iterate(storage.Query{
			Factory:      func() storage.Item { return new(obj1) },
			ItemProperty: storage.QueryItem,
		}, func(r storage.Result) (bool, error) {
			if cnt++; cnt > 1 {
				t.Fatalf("Iterate(...): duplicate detected: %v", r.Entry)
			}

			want, have := item, r.Entry
			if diff := cmp.Diff(want, have); diff != "" {
				t.Errorf("Iterate(...): unexpected result: (-want +have):\n%s", diff)
			}
			return false, nil
		})
		if err != nil {
			t.Fatalf("Iterate(...): unexpected error: %v", err)
		}
	})

	t.Run("only last ops are of interest", func(t *testing.T) {
		if err := bs.Put(item); err != nil {
			t.Fatalf("Put(...): unexpected error: %v", err)
		}

		batch, err := bs.Batch(context.Background())
		if err != nil {
			t.Fatalf("Batch(...): unexpected error: %v", err)
		}

		if err := batch.Put(item); err != nil {
			t.Fatalf("Put(...): unexpected error: %v", err)
		}
		if err := batch.Delete(item); err != nil {
			t.Fatalf("Delete(...): unexpected error: %v", err)
		}

		if err := batch.Commit(); err != nil {
			t.Fatalf("Commit(): unexpected error: %v", err)
		}

		err = bs.Iterate(storage.Query{
			Factory:      func() storage.Item { return new(obj1) },
			ItemProperty: storage.QueryItem,
		}, func(r storage.Result) (bool, error) {
			t.Fatalf("expected empty store, got %v", r.Entry)
			return true, nil
		})
		if err != nil {
			t.Fatal("iterate", err)
		}
	})

	t.Run("batch not reusable after commit", func(t *testing.T) {
		batch, err := bs.Batch(context.Background())
		if err != nil {
			t.Fatalf("Batch(...): unexpected error: %v", err)
		}
		if err := batch.Commit(); err != nil {
			t.Fatalf("Commit(): unexpected error: %v", err)
		}
		if err := batch.Commit(); err == nil { // TODO: replace with sentinel error.
			t.Fatal("Commit(): expected error; have none")
		}
	})

	t.Run("batch not usable with expired context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		batch, err := bs.Batch(ctx)
		if err != nil {
			t.Fatalf("Batch(...): unexpected error: %v", err)
		}

		if err := batch.Put(item); err != nil {
			t.Fatalf("Put(...): unexpected error: %v", err)
		}

		cancel()
		have := batch.Commit()
		want := context.Canceled
		if !errors.Is(have, want) {
			t.Fatalf("Commit(): want error: %v; have error: %v", want, have)
		}
	})
}