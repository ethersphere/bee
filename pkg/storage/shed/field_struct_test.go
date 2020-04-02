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

package shed

import (
	"context"
	"testing"

	"github.com/ethersphere/bee/pkg/storage"
)

// TestStructField validates put and get operations
// of the StructField.
func TestStructField(t *testing.T) {
	db, cleanupFunc := newTestDiskDB(t)
	defer cleanupFunc()
	ctx := context.Background()
	complexField, err := db.NewStructField(ctx, "complex-field")
	if err != nil {
		t.Fatal(err)
	}

	type complexStructure struct {
		A string
	}

	t.Run("get empty", func(t *testing.T) {
		var s complexStructure
		err := complexField.Get(ctx, &s)
		if err != storage.ErrNotFound {
			t.Fatalf("got error %v, want %v", err, storage.ErrNotFound)
		}
		want := ""
		if s.A != want {
			t.Errorf("got string %q, want %q", s.A, want)
		}
	})

	t.Run("put", func(t *testing.T) {
		want := complexStructure{
			A: "simple string value",
		}
		err = complexField.Put(ctx, want)
		if err != nil {
			t.Fatal(err)
		}
		var got complexStructure
		err = complexField.Get(ctx, &got)
		if err != nil {
			t.Fatal(err)
		}
		if got.A != want.A {
			t.Errorf("got string %q, want %q", got.A, want.A)
		}

		t.Run("overwrite", func(t *testing.T) {
			want := complexStructure{
				A: "overwritten string value",
			}
			err = complexField.Put(ctx, want)
			if err != nil {
				t.Fatal(err)
			}
			var got complexStructure
			err = complexField.Get(ctx, &got)
			if err != nil {
				t.Fatal(err)
			}
			if got.A != want.A {
				t.Errorf("got string %q, want %q", got.A, want.A)
			}
		})
	})

	t.Run("put in batch", func(t *testing.T) {
		batch := db.Store.GetBatch(true)
		want := complexStructure{
			A: "simple string batch value",
		}
		err = complexField.PutInBatch(batch, want)
		if err != nil {
			t.Fatal(err)
		}
		err = db.Store.WriteBatch(batch)
		if err != nil {
			t.Fatal(err)
		}
		var got complexStructure
		err := complexField.Get(ctx, &got)
		if err != nil {
			t.Fatal(err)
		}
		if got.A != want.A {
			t.Errorf("got string %q, want %q", got, want)
		}

		t.Run("overwrite", func(t *testing.T) {
			batch := db.Store.GetBatch(true)
			want := complexStructure{
				A: "overwritten string batch value",
			}
			err = complexField.PutInBatch(batch, want)
			if err != nil {
				t.Fatal(err)
			}
			err = db.Store.WriteBatch(batch)
			if err != nil {
				t.Fatal(err)
			}
			var got complexStructure
			err := complexField.Get(ctx, &got)
			if err != nil {
				t.Fatal(err)
			}
			if got.A != want.A {
				t.Errorf("got string %q, want %q", got, want)
			}
		})
	})
}
