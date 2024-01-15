// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package leveldbstore_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/leveldbstore"
	"github.com/ethersphere/bee/pkg/storage/storageutil"
	"github.com/google/go-cmp/cmp"
)

type obj struct {
	Key string
	Val []byte
}

func (o *obj) ID() string                 { return o.Key }
func (*obj) Namespace() string            { return "obj" }
func (o *obj) Marshal() ([]byte, error)   { return json.Marshal(o) }
func (o *obj) Unmarshal(buf []byte) error { return json.Unmarshal(buf, o) }
func (o *obj) Clone() storage.Item        { return &obj{Key: o.Key, Val: slices.Clone(o.Val)} }
func (o *obj) String() string             { return storageutil.JoinFields(o.Namespace(), o.ID()) }

func TestTxStore_Recovery(t *testing.T) {
	t.Parallel()

	store, err := leveldbstore.New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	txStore := leveldbstore.NewTxStore(store)
	t.Cleanup(func() {
		if err := txStore.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	})

	objects := make([]*obj, 10)
	for i := range objects {
		objects[i] = &obj{
			Key: fmt.Sprintf("Key-%d", i),
			Val: []byte(fmt.Sprintf("value-%d", i)),
		}
	}

	// Sore half of the objects within a transaction and commit it.
	tx := txStore.NewTx(storage.NewTxState(context.TODO()))
	for i := 0; i < len(objects)/2; i++ {
		if err := tx.Put(objects[i]); err != nil {
			t.Fatalf("put %d: %v", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Delete the first stored half of the objects and store
	// the other half and don't commit or revert the transaction.
	tx = txStore.NewTx(storage.NewTxState(context.TODO()))
	for i := 0; i < len(objects)/2; i++ {
		if err := tx.Delete(objects[i]); err != nil {
			t.Fatalf("put %d: %v", i, err)
		}
	}
	for i := len(objects) / 2; i < len(objects); i++ {
		if err := tx.Put(objects[i]); err != nil {
			t.Fatalf("put %d: %v", i, err)
		}
	}
	// Do not commit or rollback the transaction as
	// if the process crashes and attempt to recover.
	if err := txStore.Recover(); err != nil {
		t.Fatalf("recover: %v", err)
	}

	// Check that the store is in the state we expect.
	var (
		have []*obj
		want = objects[:len(objects)/2]
	)
	if err := txStore.Iterate(
		storage.Query{
			Factory:      func() storage.Item { return new(obj) },
			ItemProperty: storage.QueryItem,
		},
		func(r storage.Result) (bool, error) {
			have = append(have, r.Entry.(*obj))
			return false, nil
		},
	); err != nil {
		t.Fatalf("iterate: %v", err)
	}
	if diff := cmp.Diff(want, have); diff != "" {
		t.Fatalf("recovered store data mismatch (-want +have):\n%s", diff)
	}
}
