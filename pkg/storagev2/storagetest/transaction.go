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
	"github.com/ethersphere/bee/pkg/swarm"
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

// checkTxStoreFinishedTxInvariants check if all the store operations behave
// as expected after the transaction has been committed or rolled back.
func checkTxStoreFinishedTxInvariants(t *testing.T, store storage.TxStore) {
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

// TestTxStore provides correctness testsuite for storage.TxStore interface.
func TestTxStore(t *testing.T, store storage.TxStore) {
	t.Helper()

	t.Cleanup(func() {
		var closed int32
		time.AfterFunc(100*time.Millisecond, func() {
			if atomic.LoadInt32(&closed) == 0 {
				t.Fatal("store did not close")
			}
		})
		if err := store.Close(); err != nil {
			t.Fatalf("Close(): unexpected error: %v", err)
		}
		atomic.StoreInt32(&closed, 1)
	})

	t.Run("commit empty", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		tx := store.NewTx(storage.NewTxState(ctx))

		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit(): unexpected error: %v", err)
		}

		checkTxStoreFinishedTxInvariants(t, tx)
	})

	t.Run("commit", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		objects := []*object{
			{id: "0001", data: []byte("data1")},
			{id: "0002", data: []byte("data2")},
			{id: "0003", data: []byte("data3")},
		}

		t.Run("add new objects", func(t *testing.T) {
			tx := store.NewTx(storage.NewTxState(ctx))

			initStore(t, tx, objects...)

			if err := tx.Commit(); err != nil {
				t.Fatalf("Commit(): unexpected error: %v", err)
			}

			for _, o := range objects {
				err := store.Get(&object{id: o.id})
				if err != nil {
					t.Fatalf("Get(%q): unexpected error: %v", o.id, err)
				}
			}

			checkTxStoreFinishedTxInvariants(t, tx)
		})

		t.Run("delete existing objects", func(t *testing.T) {
			tx := store.NewTx(storage.NewTxState(ctx))

			for _, o := range objects {
				if err := tx.Delete(o); err != nil {
					t.Fatalf("Delete(%q): unexpected error: %v", o.id, err)
				}
			}
			if err := tx.Commit(); err != nil {
				t.Fatalf("Commit(): unexpected error: %v", err)
			}
			want := storage.ErrNotFound
			for _, o := range objects {
				have := store.Get(&object{id: o.id})
				if !errors.Is(have, want) {
					t.Fatalf("Get(%q):\n\thave: %v\n\twant: %v", o.id, want, have)
				}
			}

			checkTxStoreFinishedTxInvariants(t, tx)
		})
	})

	t.Run("rollback empty", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		tx := store.NewTx(storage.NewTxState(ctx))

		if err := tx.Rollback(); err != nil {
			t.Fatalf("Rollback(): unexpected error: %v", err)
		}

		checkTxStoreFinishedTxInvariants(t, tx)
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
				t.Fatalf("Get(%q):\n\thave: %v\n\twant: %v", o.id, want, have)
			}
		}

		checkTxStoreFinishedTxInvariants(t, tx)
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

		checkTxStoreFinishedTxInvariants(t, tx)
	})
}

// initChunkStore initializes the given store with the given chunks.
func initChunkStore(t *testing.T, store storage.ChunkStore, chunks ...swarm.Chunk) {
	t.Helper()

	ctx := context.Background()
	for _, chunk := range chunks {
		if err := store.Put(ctx, chunk); err != nil {
			t.Fatalf("Put(%q): unexpected error: %v", chunk.Address(), err)
		}
	}
}

// checkTxChunkStoreFinishedTxInvariants check if all the store operations behave
// as expected after the transaction has been committed or rolled back.
func checkTxChunkStoreFinishedTxInvariants(t *testing.T, store storage.TxChunkStore) {
	t.Helper()

	ctx := context.Background()
	want := storage.ErrTxDone
	o007 := swarm.NewChunk(swarm.NewAddress([]byte("007")), []byte("Hello, World!"))

	if chunk, have := store.Get(ctx, o007.Address()); !errors.Is(have, want) || chunk != nil {
		t.Fatalf("Get(...)\n\thave: %v, %v\n\twant: <nil>, %v", chunk, have, want)
	}

	if have := store.Put(ctx, o007); !errors.Is(have, want) {
		t.Fatalf("Put(...):\n\thave: %v\n\twant: %v", have, want)
	}

	if have := store.Delete(ctx, o007.Address()); !errors.Is(have, want) {
		t.Fatalf("Delete(...):\n\thave: %v\n\twant: %v", have, want)
	}

	if _, have := store.Has(ctx, swarm.ZeroAddress); !errors.Is(have, want) {
		t.Fatalf("Has(...):\n\thave: %v\n\twant: %v", have, want)
	}

	if have := store.Iterate(ctx, func(_ swarm.Chunk) (stop bool, err error) {
		return false, nil
	}); !errors.Is(have, want) {
		t.Fatalf("Iterate(...):\n\thave: %v\n\twant: %v", have, want)
	}

	if have, want := store.Commit(), storage.ErrTxDone; !errors.Is(have, want) {
		t.Fatalf("Commit():\n\thave: %v\n\twant: %v", have, want)
	}

	if have, want := store.Rollback(), storage.ErrTxDone; !errors.Is(have, want) {
		t.Fatalf("Rollback():\n\thave: %v\n\twant: %v", have, want)
	}
}

// TestTxChunkStore provides correctness testsuite for storage.TxChunkStore interface.
func TestTxChunkStore(t *testing.T, store storage.TxChunkStore) {
	t.Helper()

	t.Cleanup(func() {
		var closed int32
		time.AfterFunc(100*time.Millisecond, func() {
			if atomic.LoadInt32(&closed) == 0 {
				t.Fatal("store did not close")
			}
		})
		if err := store.Close(); err != nil {
			t.Fatalf("Close(): unexpected error: %v", err)
		}
		atomic.StoreInt32(&closed, 1)
	})

	t.Run("commit empty", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		tx := store.NewTx(storage.NewTxState(ctx))

		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit(): unexpected error: %v", err)
		}

		checkTxChunkStoreFinishedTxInvariants(t, tx)
	})

	t.Run("commit", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		chunks := []swarm.Chunk{
			swarm.NewChunk(swarm.NewAddress([]byte("0001")), []byte("data1")),
			swarm.NewChunk(swarm.NewAddress([]byte("0002")), []byte("data2")),
			swarm.NewChunk(swarm.NewAddress([]byte("0003")), []byte("data3")),
		}

		t.Run("add new chunks", func(t *testing.T) {
			tx := store.NewTx(storage.NewTxState(ctx))

			initChunkStore(t, tx, chunks...)

			if err := tx.Commit(); err != nil {
				t.Fatalf("Commit(): unexpected error: %v", err)
			}

			for _, want := range chunks {
				have, err := store.Get(context.Background(), want.Address())
				if err != nil {
					t.Fatalf("Get(%q): unexpected error: %v", want.Address(), err)
				}
				if !have.Equal(want) {
					t.Fatalf("Get(%q): \n\thave: %v\n\twant: %v", want.Address(), have, want)
				}
			}

			checkTxChunkStoreFinishedTxInvariants(t, tx)
		})

		t.Run("delete existing chunks", func(t *testing.T) {
			tx := store.NewTx(storage.NewTxState(ctx))

			for _, chunk := range chunks {
				if err := tx.Delete(context.Background(), chunk.Address()); err != nil {
					t.Fatalf("Delete(%q): unexpected error: %v", chunk.Address(), err)
				}
			}
			if err := tx.Commit(); err != nil {
				t.Fatalf("Commit(): unexpected error: %v", err)
			}
			want := storage.ErrNotFound
			for _, ch := range chunks {
				chunk, have := store.Get(context.Background(), ch.Address())
				if !errors.Is(have, want) || chunk != nil {
					t.Fatalf("Get(...)\n\thave: %v, %v\n\twant: <nil>, %v", chunk, have, want)
				}
			}

			checkTxChunkStoreFinishedTxInvariants(t, tx)
		})
	})

	t.Run("rollback empty", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		tx := store.NewTx(storage.NewTxState(ctx))

		if err := tx.Rollback(); err != nil {
			t.Fatalf("Rollback(): unexpected error: %v", err)
		}

		checkTxChunkStoreFinishedTxInvariants(t, tx)
	})

	t.Run("rollback added chunks", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		tx := store.NewTx(storage.NewTxState(ctx))

		chunks := []swarm.Chunk{
			swarm.NewChunk(swarm.NewAddress([]byte("0001")), []byte("data1")),
			swarm.NewChunk(swarm.NewAddress([]byte("0002")), []byte("data2")),
			swarm.NewChunk(swarm.NewAddress([]byte("0003")), []byte("data3")),
		}
		initChunkStore(t, tx, chunks...)

		if err := tx.Rollback(); err != nil {
			t.Fatalf("Rollback(): unexpected error: %v", err)
		}

		want := storage.ErrNotFound
		for _, ch := range chunks {
			chunk, have := store.Get(context.Background(), ch.Address())
			if !errors.Is(have, want) || chunk != nil {
				t.Fatalf("Get(...)\n\thave: %v, %v\n\twant: <nil>, %v", chunk, have, want)
			}
		}

		checkTxChunkStoreFinishedTxInvariants(t, tx)
	})

	t.Run("rollback removed chunks", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		tx := store.NewTx(storage.NewTxState(ctx))
		chunks := []swarm.Chunk{
			swarm.NewChunk(swarm.NewAddress([]byte("0001")), []byte("data1")),
			swarm.NewChunk(swarm.NewAddress([]byte("0002")), []byte("data2")),
			swarm.NewChunk(swarm.NewAddress([]byte("0003")), []byte("data3")),
		}
		initChunkStore(t, tx, chunks...)
		if err := tx.Commit(); err != nil {
			t.Fatalf("Commit(): unexpected error: %v", err)
		}

		tx = store.NewTx(storage.NewTxState(ctx))
		for _, ch := range chunks {
			if err := tx.Delete(context.Background(), ch.Address()); err != nil {
				t.Fatalf("Delete(%q): unexpected error: %v", ch.Address(), err)
			}
		}
		if err := tx.Rollback(); err != nil {
			t.Fatalf("Rollback(): unexpected error: %v", err)
		}
		for _, want := range chunks {
			have, err := store.Get(context.Background(), want.Address())
			if err != nil {
				t.Fatalf("Get(%q): unexpected error: %v", want.Address(), err)
			}
			if !have.Equal(want) {
				t.Fatalf("Get(%q): \n\thave: %v\n\twant: %v", want.Address(), have, want)
			}
		}

		checkTxChunkStoreFinishedTxInvariants(t, tx)
	})
}
