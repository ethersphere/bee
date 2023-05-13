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
	"bytes"
	"testing"
)

// TestDB_schemaFieldKey validates correctness of schemaFieldKey.
func TestDB_schemaFieldKey(t *testing.T) {
	t.Parallel()

	t.Run("empty name or type", func(t *testing.T) {
		t.Parallel()

		db := newTestDB(t)
		_, err := db.schemaFieldKey("", "")
		if err == nil {
			t.Error("error not returned, but expected")
		}
		_, err = db.schemaFieldKey("", "type")
		if err == nil {
			t.Error("error not returned, but expected")
		}

		_, err = db.schemaFieldKey("test", "")
		if err == nil {
			t.Error("error not returned, but expected")
		}
	})

	t.Run("same field", func(t *testing.T) {
		t.Parallel()

		db := newTestDB(t)
		key1, err := db.schemaFieldKey("test", "undefined")
		if err != nil {
			t.Fatal(err)
		}

		key2, err := db.schemaFieldKey("test", "undefined")
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(key1, key2) {
			t.Errorf("schema keys for the same field name are not the same: %q, %q", string(key1), string(key2))
		}
	})

	t.Run("different fields", func(t *testing.T) {
		t.Parallel()

		db := newTestDB(t)
		key1, err := db.schemaFieldKey("test1", "undefined")
		if err != nil {
			t.Fatal(err)
		}

		key2, err := db.schemaFieldKey("test2", "undefined")
		if err != nil {
			t.Fatal(err)
		}

		if bytes.Equal(key1, key2) {
			t.Error("schema keys for the same field name are the same, but must not be")
		}
	})

	t.Run("same field name different types", func(t *testing.T) {
		t.Parallel()

		db := newTestDB(t)
		_, err := db.schemaFieldKey("the-field", "one-type")
		if err != nil {
			t.Fatal(err)
		}

		_, err = db.schemaFieldKey("the-field", "another-type")
		if err == nil {
			t.Error("error not returned, but expected")
		}
	})
}

// TestDB_schemaIndexPrefix validates correctness of schemaIndexPrefix.
func TestDB_schemaIndexPrefix(t *testing.T) {
	t.Parallel()

	t.Run("same name", func(t *testing.T) {
		t.Parallel()

		db := newTestDB(t)
		id1, err := db.schemaIndexPrefix("test")
		if err != nil {
			t.Fatal(err)
		}

		id2, err := db.schemaIndexPrefix("test")
		if err != nil {
			t.Fatal(err)
		}

		if id1 != id2 {
			t.Errorf("schema keys for the same field name are not the same: %v, %v", id1, id2)
		}
	})

	t.Run("different names", func(t *testing.T) {
		t.Parallel()

		db := newTestDB(t)
		id1, err := db.schemaIndexPrefix("test1")
		if err != nil {
			t.Fatal(err)
		}

		id2, err := db.schemaIndexPrefix("test2")
		if err != nil {
			t.Fatal(err)
		}

		if id1 == id2 {
			t.Error("schema ids for the same index name are the same, but must not be")
		}
	})
}

// TestDB_RenameIndex checks if index name is correctly changed.
func TestDB_RenameIndex(t *testing.T) {
	t.Parallel()

	t.Run("empty names", func(t *testing.T) {
		t.Parallel()

		db := newTestDB(t)

		// empty names
		renamed, err := db.RenameIndex("", "")
		if err == nil {
			t.Error("error not returned, but expected")
		}
		if renamed {
			t.Fatal("index should not be renamed")
		}

		// empty index name
		renamed, err = db.RenameIndex("", "new")
		if err == nil {
			t.Error("error not returned, but expected")
		}
		if renamed {
			t.Fatal("index should not be renamed")
		}

		// empty new index name
		renamed, err = db.RenameIndex("current", "")
		if err == nil {
			t.Error("error not returned, but expected")
		}
		if renamed {
			t.Fatal("index should not be renamed")
		}
	})

	t.Run("same names", func(t *testing.T) {
		t.Parallel()

		db := newTestDB(t)

		renamed, err := db.RenameIndex("index1", "index1")
		if err != nil {
			t.Error(err)
		}
		if renamed {
			t.Fatal("index should not be renamed")
		}
	})

	t.Run("unknown name", func(t *testing.T) {
		t.Parallel()

		db := newTestDB(t)

		renamed, err := db.RenameIndex("index1", "index1new")
		if err != nil {
			t.Error(err)
		}
		if renamed {
			t.Fatal("index should not be renamed")
		}
	})

	t.Run("valid names", func(t *testing.T) {
		t.Parallel()

		db := newTestDB(t)

		// initial indexes
		key1, err := db.schemaIndexPrefix("index1")
		if err != nil {
			t.Fatal(err)
		}
		key2, err := db.schemaIndexPrefix("index2")
		if err != nil {
			t.Fatal(err)
		}

		// name the first one
		renamed, err := db.RenameIndex("index1", "index1new")
		if err != nil {
			t.Fatal(err)
		}
		if !renamed {
			t.Fatal("index not renamed")
		}

		// validate that the index key stays the same
		key1same, err := db.schemaIndexPrefix("index1new")
		if err != nil {
			t.Fatal(err)
		}
		if key1 != key1same {
			t.Fatal("indexes renamed, but keys are not the same")
		}

		// validate that the independent index is not changed
		key2same, err := db.schemaIndexPrefix("index2")
		if err != nil {
			t.Fatal(err)
		}
		if key2 != key2same {
			t.Fatal("independent index key has changed")
		}

		// validate that it is safe to create a new index with previous name
		key1renew, err := db.schemaIndexPrefix("index1")
		if err != nil {
			t.Fatal(err)
		}
		if key1 == key1renew {
			t.Fatal("renewed index and the original one have the same key")
		}
		if key2 == key1renew {
			t.Fatal("renewed index and the independent one have the same key")
		}
	})
}
