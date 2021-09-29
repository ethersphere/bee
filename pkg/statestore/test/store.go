// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/storage"
)

const (
	key1 = "key1" // stores the serialized type
	key2 = "key2" // stores a json array
)

var (
	value1 = &Serializing{value: "value1"}
	value2 = []string{"a", "b", "c"}
)

type Serializing struct {
	value           string
	marshalCalled   bool
	unmarshalCalled bool
}

func (st *Serializing) MarshalBinary() (data []byte, err error) {
	d := []byte(st.value)
	st.marshalCalled = true

	return d, nil
}

func (st *Serializing) UnmarshalBinary(data []byte) (err error) {
	st.value = string(data)
	st.unmarshalCalled = true
	return nil
}

// RunPersist is a specific test case for the persistent state store.
// It tests that values persist across sessions.
func RunPersist(t *testing.T, f func(t *testing.T, dir string) storage.StateStorer) {
	t.Helper()
	dir, err := os.MkdirTemp("", "statestore_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	store := f(t, dir)

	// insert some values
	insert(t, store, "some_prefix", 1000)

	// test that the iterator works
	testStoreIterator(t, store, "some_prefix", 1000)

	// close the store
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// bootstrap with the same old dir
	persistedStore := f(t, dir)
	defer persistedStore.Close()

	// test that the iterator works
	testStoreIterator(t, persistedStore, "some_prefix", 1000)

	// insert some more random entries
	insert(t, persistedStore, "some_other_prefix", 1000)

	// check again
	testStoreIterator(t, persistedStore, "some_other_prefix", 1000)
}

func Run(t *testing.T, f func(t *testing.T) storage.StateStorer) {
	t.Helper()

	t.Run("test_put_get", func(t *testing.T) { testPutGet(t, f) })
	t.Run("test_delete", func(t *testing.T) { testDelete(t, f) })
	t.Run("test_iterator", func(t *testing.T) { testIterator(t, f) })
}

func testDelete(t *testing.T, f func(t *testing.T) storage.StateStorer) {
	t.Helper()

	// create a store
	store := f(t)

	// insert some values
	insertValues(t, store, key1, key2, value1, value2)

	// check that the persisted values match
	testPersistedValues(t, store, key1, key2, value1, value2)

	err := store.Delete(key1)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Delete(key2)
	if err != nil {
		t.Fatal(err)
	}

	// check that the store is empty
	testEmpty(t, store)
}

func testPutGet(t *testing.T, f func(t *testing.T) storage.StateStorer) {
	t.Helper()

	// create a store
	store := f(t)

	// insert some values
	insertValues(t, store, key1, key2, value1, value2)

	// check that the persisted values match
	testPersistedValues(t, store, key1, key2, value1, value2)
}

func testIterator(t *testing.T, f func(t *testing.T) storage.StateStorer) {
	t.Helper()

	// create a store
	store := f(t)

	// insert some values
	insert(t, store, "some_prefix", 1000)

	// test that the iterator works
	testStoreIterator(t, store, "some_prefix", 1000)
	testStoreIterator(t, store, "no_prefix", 0)
}

func insertValues(t *testing.T, store storage.StateStorer, key1, key2 string, value1 *Serializing, value2 []string) {
	t.Helper()
	err := store.Put(key1, value1)
	if err != nil {
		t.Fatal(err)
	}

	if !value1.marshalCalled {
		t.Fatal("binaryMarshaller not called on serialized type")
	}

	err = store.Put(key2, value2)
	if err != nil {
		t.Fatal(err)
	}
}

func insert(t *testing.T, store storage.StateStorer, prefix string, count int) {
	t.Helper()

	for i := 0; i < count; i++ {
		k := prefix + fmt.Sprint(i)

		err := store.Put(k, i)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func testPersistedValues(t *testing.T, store storage.StateStorer, key1, key2 string, value1 *Serializing, value2 []string) {
	t.Helper()

	v := &Serializing{}
	err := store.Get(key1, v)
	if err != nil {
		t.Fatal(err)
	}

	if !v.unmarshalCalled {
		t.Fatal("unmarshaler not called")
	}

	if v.value != value1.value {
		t.Fatalf("expected persisted to be %s but got %s", value1.value, v.value)
	}

	s := []string{}
	err = store.Get(key2, &s)
	if err != nil {
		t.Fatal(err)
	}

	for i, ss := range value2 {
		if s[i] != ss {
			t.Fatalf("deserialized data mismatch. expected %s but got %s", ss, s[i])
		}
	}
}

func testStoreIterator(t *testing.T, store storage.StateStorer, prefix string, size int) {
	t.Helper()

	matching := 0
	entriesIterFunction := func(key []byte, value []byte) (stop bool, err error) {
		k := string(key)
		if !strings.HasPrefix(k, prefix) {
			return true, fmt.Errorf("iterator called callback with wrong key prefix. key: %s expected prefix: %s", k, prefix)
		}
		matching++
		return false, nil
	}

	err := store.Iterate(prefix, entriesIterFunction)
	if err != nil {
		t.Fatal(err)
	}

	if matching != size {
		t.Fatalf("entry number mismatch. want %d got %d", size, matching)
	}
}

func testEmpty(t *testing.T, store storage.StateStorer) {
	t.Helper()

	testStoreIterator(t, store, "", 1)
}
