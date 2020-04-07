package test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/pkg/statestore"
	"github.com/ethersphere/bee/pkg/storage"
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

func TestStore(t *testing.T) {
	dir, err := ioutil.TempDir("", "statestore_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	const (
		key1 = "key1" // stores the serialized type
		key2 = "key2" // stores a json array
	)

	var (
		value1 = &Serializing{value: "value1"}
		value2 = []string{"a", "b", "c"}
	)

	// create a new persisted store
	store, err := statestore.New(dir)
	if err != nil {
		t.Fatal(err)
	}

	// insert some values
	insertValues(t, store, key1, key2, value1, value2)

	// close the persisted store
	store.Close()

	// bootstrap a new store with the persisted data
	persistedStore, err := statestore.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer persistedStore.Close()

	// check that the persisted values match
	testPersistedValues(t, persistedStore, key1, key2, value1, value2)

	// test that the iterator works
	testStoreIterator(t, persistedStore)
}

func insertValues(t *testing.T, store storage.StateStorer, key1, key2 string, value1 *Serializing, value2 []string) {
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

func testPersistedValues(t *testing.T, store storage.StateStorer, key1, key2 string, value1 *Serializing, value2 []string) {
	v := &Serializing{}
	err := store.Get(key1, v)
	if err != nil {
		t.Fatal(err)
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

func testStoreIterator(t *testing.T, store storage.StateStorer) {
	storePrefix := "test_"
	err := store.Put(storePrefix+"key1", "value1")
	if err != nil {
		t.Fatal(err)
	}

	// do not include prefix in one of the entries
	err = store.Put("key2", "value2")
	if err != nil {
		t.Fatal(err)
	}

	err = store.Put(storePrefix+"key3", "value3")
	if err != nil {
		t.Fatal(err)
	}

	entries := make(map[string]string)

	entriesIterFunction := func(key []byte, value []byte) (stop bool, err error) {
		var entry string
		err = json.Unmarshal(value, &entry)
		if err != nil {
			t.Fatal(err)
		}
		entries[string(key)] = entry
		return stop, err
	}

	err = store.Iterate(storePrefix, entriesIterFunction)
	if err != nil {
		t.Fatal(err)
	}

	expectedEntries := map[string]string{"test_key1": "value1", "test_key3": "value3"}

	if !reflect.DeepEqual(entries, expectedEntries) {
		t.Fatalf("expected store entries to be %v, are %v instead", expectedEntries, entries)
	}
}
