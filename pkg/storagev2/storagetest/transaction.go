// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	//"github.com/ethersphere/bee/pkg/storagev2/leveldbstore"
)

var _ storage.Item = (*object)(nil)

// object is a simple struct that implements
// the storage.Item interface.
type object struct {
	id   string // 4 bytes.
	data []byte
}

func (i object) ID() string      { return i.id }
func (object) Namespace() string { return "object" }

func (i object) Marshal() ([]byte, error) {
	buf := make([]byte, 4+len(i.data))
	copy(buf[:4], i.id)
	copy(buf[4:], i.data)
	return buf, nil
}

func (i *object) Unmarshal(buf []byte) error {
	if len(buf) < 4 {
		return errors.New("invalid length")
	}
	i.id = string(buf[:4])
	i.data = make([]byte, len(buf)-4)
	copy(i.data, buf[4:])
	return nil
}

// initStore initializes the given store with the given objects.
func initStore(t *testing.T, store storage.Store, objects ...*object) {
	t.Helper()

	for _, o := range objects {
		if err := store.Put(o); err != nil {
			t.Fatalf("Put(%q): unexpected error: %v", o.id, err)
		}
	}
}

// checkFinishedTxInvariants check if all the store operations behave
// as expected after the transaction has been committed or rolled back.
func checkFinishedTxInvariants(t *testing.T, store storage.TxStore) {
	t.Helper()

	o007 := &object{id: "007", data: []byte("Hello, World!")}
	want := storage.ErrTxDone

	if have := store.Get(o007); !errors.Is(have, want) {
		t.Fatalf("Get(...):\n\thave: %v\n\twant: %v", have, want)
	}

	if _, have := store.Has(o007); !errors.Is(have, want) {
		t.Fatalf("Has(...):\n\thave: %v\n\twant: %v", have, want)
	}

	if _, have := store.GetSize(o007); !errors.Is(have, want) {
		t.Fatalf("GetSize(...):\n\thave: %v\n\twant: %v", have, want)
	}

	if have := store.Iterate(storage.Query{}, nil); !errors.Is(have, want) {
		t.Fatalf("Iterate(...):\n\thave: %v\n\twant: %v", have, want)
	}

	if _, have := store.Count(o007); !errors.Is(have, want) {
		t.Fatalf("Count(...):\n\thave: %v\n\twant: %v", have, want)
	}

	if have := store.Put(o007); !errors.Is(have, want) {
		t.Fatalf("Put(...):\n\thave: %v\n\twant: %v", have, want)
	}

	if have := store.Delete(o007); !errors.Is(have, want) {
		t.Fatalf("Delete(...):\n\thave: %v\n\twant: %v", have, want)
	}

	if have, want := store.Commit(), storage.ErrTxDone; !errors.Is(have, want) {
		t.Fatalf("Commit():\n\thave: %v\n\twant: %v", have, want)
	}

	if have, want := store.Rollback(), storage.ErrTxDone; !errors.Is(have, want) {
		t.Fatalf("Rollback():\n\thave: %v\n\twant: %v", have, want)
	}
}

// TestTxStore provides correctness testsuite for TxStore interface.
func TestTxStore(t *testing.T, store storage.TxStore) {
	t.Helper()

	t.Cleanup(func() {
		var closed int32
		time.AfterFunc(time.Millisecond, func() {
			if atomic.LoadInt32(&closed) == 0 {
				t.Fatal("store did not close")
			}
		})
		if err := store.Close(); err != nil {
			t.Fatalf("Close(): unexpected error: %v", err)
		}
		atomic.StoreInt32(&closed, 1)
	})

	// TODO: commit tests

	t.Run("rollback empty", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		tx := store.NewTx(storage.NewTxState(ctx))

		if err := tx.Rollback(); err != nil {
			t.Fatalf("Rollback(): unexpected error: %v", err)
		}

		checkFinishedTxInvariants(t, tx)
	})

	t.Run("rollback added objects", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		tx := store.NewTx(storage.NewTxState(ctx))

		objects := []*object{
			{id: "0001", data: []byte("data1")},
			{id: "0002", data: []byte("data2")},
			{id: "0003", data: []byte("data3")},
		}
		initStore(t, tx, objects...)

		if err := tx.Rollback(); err != nil {
			t.Fatalf("Rollback(): unexpected error: %v", err)
		}

		want := storage.ErrNotFound
		for _, o := range objects {
			have := store.Get(&object{id: o.id})
			if !errors.Is(have, want) {
				t.Fatalf("Get(%q): want: %v; have: %v", o.id, want, have)
			}
		}

		checkFinishedTxInvariants(t, tx)
	})

	t.Run("rollback removed objects", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		tx := store.NewTx(storage.NewTxState(ctx))
		objects := []*object{
			{id: "0001", data: []byte("data1")},
			{id: "0002", data: []byte("data2")},
			{id: "0003", data: []byte("data3")},
		}
		initStore(t, tx, objects...)
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit(): unexpected error: %v", err)
		}

		tx = store.NewTx(storage.NewTxState(ctx))
		for _, o := range objects {
			if err := tx.Delete(o); err != nil {
				t.Fatalf("Delete(%q): unexpected error: %v", o.id, err)
			}
		}
		if err := tx.Rollback(); err != nil {
			t.Fatalf("Rollback(): unexpected error: %v", err)
		}
		for _, want := range objects {
			have := &object{id: want.id}
			if err := store.Get(have); err != nil {
				t.Errorf("Get(%q): unexpected error: %v", want.id, err)
			}
			if have.id != want.id {
				t.Errorf("Get(%q):\n\thave: %q\n\twant: %q", want.id, have.id, want.id)
			}
			if !bytes.Equal(have.data, want.data) {
				t.Errorf("Get(%q):\n\thave: %x\n\twant: %x", want.id, have.data, want.data)
			}
		}

		checkFinishedTxInvariants(t, tx)
	})
}
