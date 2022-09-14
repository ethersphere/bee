// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"context"
	"errors"
	"testing"

	storage "github.com/ethersphere/bee/pkg/storagev2"
)

func TestBatch(t *testing.T, s storage.Store) {
	t.Run("duplicates are rejected", func(t *testing.T) {
		b, _ := s.Batch(context.Background())

		item1 := &obj1{
			Id: "id1",
		}

		_ = b.Put(item1)
		_ = b.Put(item1)

		_ = b.Commit()

		_ = s.Iterate(storage.Query{
			Factory:       func() storage.Item { return new(obj1) },
			ItemAttribute: storage.QueryItem,
		}, func(r storage.Result) (bool, error) {
			if r.Entry.ID() != item1.ID() {
				t.Fatalf("expected id %s, got %s", item1.ID(), r.Entry.ID())
			}
			return true, nil
		})
	})

	t.Run("delete first removes from batch then from store", func(t *testing.T) {
		item1 := &obj1{
			Id: "id1",
		}
		_ = s.Put(item1)

		b, _ := s.Batch(context.Background())
		item2 := &obj1{
			Id: "id2",
		}
		_ = b.Put(item2)

		_ = b.Delete(item1)
		_ = b.Delete(item2)

		_ = b.Commit()

		_ = s.Iterate(storage.Query{
			Factory:       func() storage.Item { return new(obj1) },
			ItemAttribute: storage.QueryItem,
		}, func(r storage.Result) (bool, error) {
			t.Fatalf("expected empty store, got %v", r.Entry)
			return true, nil
		})
	})

	t.Run("batche not reusable after commit", func(t *testing.T) {
		b, _ := s.Batch(context.Background())
		_ = b.Commit()
		if err := b.Commit(); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("batche not usable with expired context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		b, _ := s.Batch(ctx)
		item := &obj1{
			Id: "id2",
		}
		_ = b.Put(item)

		cancel()

		if err := b.Commit(); !errors.Is(err, context.Canceled) {
			t.Fatal("expected context cancelled, got nil", err)
		}
	})
}
