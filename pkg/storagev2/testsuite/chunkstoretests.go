// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storetesting

import (
	"context"
	"errors"
	"testing"

	chunktest "github.com/ethersphere/bee/pkg/storage/testing"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
)

func RunChunkStoreCorrectnessTests(t *testing.T, st storage.ChunkStore) {
	t.Helper()

	testChunks := chunktest.GenerateTestRandomChunks(50)

	t.Run("put chunks", func(t *testing.T) {
		for _, ch := range testChunks {
			exists, err := st.Put(context.TODO(), ch)
			if err != nil {
				t.Fatalf("failed putting new chunk: %v", err)
			}
			if exists {
				t.Fatalf("expected chunk to not exist: %s", ch.Address())
			}
		}
	})

	t.Run("put existing chunks", func(t *testing.T) {
		for _, ch := range testChunks {
			exists, err := st.Put(context.TODO(), ch)
			if err != nil {
				t.Fatalf("failed putting new chunk: %v", err)
			}
			if !exists {
				t.Fatalf("expected chunk to exist: %s", ch.Address())
			}
		}
	})

	t.Run("get chunks", func(t *testing.T) {
		for _, ch := range testChunks {
			readCh, err := st.Get(context.TODO(), ch.Address())
			if err != nil {
				t.Fatalf("failed getting chunk: %v", err)
			}
			if !readCh.Equal(ch) {
				t.Fatal("read chunk doesnt match")
			}
		}
	})

	t.Run("has chunks", func(t *testing.T) {
		for _, ch := range testChunks {
			exists, err := st.Has(context.TODO(), ch.Address())
			if err != nil {
				t.Fatalf("failed getting chunk: %v", err)
			}
			if !exists {
				t.Fatalf("chunk not found: %s", ch.Address())
			}
		}
	})

	t.Run("iterate chunks", func(t *testing.T) {
		count := 0
		err := st.Iterate(context.TODO(), func(_ swarm.Chunk) (bool, error) {
			count++
			return false, nil
		})
		if err != nil {
			t.Fatalf("unexpected error while iteration: %v", err)
		}
		if count != 50 {
			t.Fatalf("unexpected no of chunks, exp: %d, found: %d", 50, count)
		}
	})

	t.Run("delete chunks", func(t *testing.T) {
		for idx, ch := range testChunks {
			// Delete all even numbered indexes along with 0
			if idx%2 == 0 {
				err := st.Delete(context.TODO(), ch.Address())
				if err != nil {
					t.Fatalf("failed deleting chunk: %v", err)
				}
			}
		}
	})

	t.Run("check deleted chunks", func(t *testing.T) {
		for idx, ch := range testChunks {
			if idx%2 == 0 {
				// Check even numbered indexes are deleted
				_, err := st.Get(context.TODO(), ch.Address())
				if !errors.Is(err, storage.ErrNotFound) {
					t.Fatalf("expected storage not found error found: %v", err)
				}
				found, err := st.Has(context.TODO(), ch.Address())
				if err != nil {
					t.Fatalf("unexpected error in Has: %v", err)
				}
				if found {
					t.Fatal("expected chunk to not be found")
				}
			} else {
				// Check rest of the entries are intact
				readCh, err := st.Get(context.TODO(), ch.Address())
				if err != nil {
					t.Fatalf("failed getting chunk: %v", err)
				}
				if !readCh.Equal(ch) {
					t.Fatal("read chunk doesnt match")
				}
				exists, err := st.Has(context.TODO(), ch.Address())
				if err != nil {
					t.Fatalf("failed getting chunk: %v", err)
				}
				if !exists {
					t.Fatalf("chunk not found: %s", ch.Address())
				}
			}
		}
	})

	t.Run("iterate chunks after delete", func(t *testing.T) {
		count := 0
		err := st.Iterate(context.TODO(), func(_ swarm.Chunk) (bool, error) {
			count++
			return false, nil
		})
		if err != nil {
			t.Fatalf("unexpected error while iteration: %v", err)
		}
		if count != 25 {
			t.Fatalf("unexpected no of chunks, exp: %d, found: %d", 25, count)
		}
	})

	t.Run("close store", func(t *testing.T) {
		err := st.Close()
		if err != nil {
			t.Fatalf("unexpected error during close: %v", err)
		}
	})
}
