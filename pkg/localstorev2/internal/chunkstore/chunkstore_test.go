// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore_test

import (
	"context"
	"errors"
	"io/fs"
	"math"
	"os"
	"testing"

	"github.com/ethersphere/bee/pkg/localstorev2/internal/chunkstore"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/sharky"
	chunktest "github.com/ethersphere/bee/pkg/storage/testing"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/inmemstore"
	"github.com/ethersphere/bee/pkg/storagev2/storagetest"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/google/go-cmp/cmp"
	"github.com/spf13/afero"
)

func TestRetrievalIndexItem_MarshalAndUnmarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		test *storagetest.ItemMarshalAndUnmarshalTest
	}{{
		name: "zero values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:       &chunkstore.RetrievalIndexItem{},
			Factory:    func() storage.Item { return new(chunkstore.RetrievalIndexItem) },
			MarshalErr: chunkstore.ErrMarshalInvalidRetrievalIndexItemAddress,
		},
	}, {
		name: "zero address",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &chunkstore.RetrievalIndexItem{
				Address: swarm.ZeroAddress,
			},
			Factory:    func() storage.Item { return new(chunkstore.RetrievalIndexItem) },
			MarshalErr: chunkstore.ErrMarshalInvalidRetrievalIndexItemAddress,
		},
	}, {
		name: "min values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &chunkstore.RetrievalIndexItem{
				Address: swarm.NewAddress(storagetest.MinAddressBytes[:]),
			},
			Factory: func() storage.Item { return new(chunkstore.RetrievalIndexItem) },
		},
	}, {
		name: "max values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &chunkstore.RetrievalIndexItem{
				Address:   swarm.NewAddress(storagetest.MaxAddressBytes[:]),
				Timestamp: math.MaxUint64,
				Location: sharky.Location{
					Shard:  math.MaxUint8,
					Slot:   math.MaxUint32,
					Length: math.MaxUint16,
				},
				RefCnt: math.MaxUint8,
			},
			Factory: func() storage.Item { return new(chunkstore.RetrievalIndexItem) },
		},
	}, {
		name: "invalid size",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &storagetest.ItemStub{
				MarshalBuf:   []byte{0xFF},
				UnmarshalBuf: []byte{0xFF},
			},
			Factory:      func() storage.Item { return new(chunkstore.RetrievalIndexItem) },
			UnmarshalErr: chunkstore.ErrUnmarshalInvalidRetrievalIndexItemSize,
		},
	}}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			storagetest.TestItemMarshalAndUnmarshal(t, tc.test)
		})
	}
}

func TestChunkStampItem_MarshalAndUnmarshal(t *testing.T) {
	t.Parallel()

	minAddress := swarm.NewAddress(storagetest.MinAddressBytes[:])
	minStamp := postage.NewStamp(make([]byte, 32), make([]byte, 8), make([]byte, 8), make([]byte, 65))
	chunk := chunktest.GenerateTestRandomChunk()

	tests := []struct {
		name string
		test *storagetest.ItemMarshalAndUnmarshalTest
	}{{
		name: "zero values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:       &chunkstore.ChunkStampItem{},
			Factory:    func() storage.Item { return new(chunkstore.ChunkStampItem) },
			MarshalErr: chunkstore.ErrMarshalInvalidChunkStampItemAddress,
		},
	}, {
		name: "zero address",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &chunkstore.ChunkStampItem{
				Address: swarm.ZeroAddress,
			},
			Factory:    func() storage.Item { return new(chunkstore.ChunkStampItem) },
			MarshalErr: chunkstore.ErrMarshalInvalidChunkStampItemAddress,
		},
	}, {
		name: "nil stamp",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &chunkstore.ChunkStampItem{
				Address: minAddress,
			},
			Factory:    func() storage.Item { return new(chunkstore.ChunkStampItem) },
			MarshalErr: chunkstore.ErrMarshalInvalidChunkStampItemStamp,
		},
	}, {
		name: "zero stamp",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &chunkstore.ChunkStampItem{
				Address: minAddress,
				Stamp:   new(postage.Stamp),
			},
			Factory:    func() storage.Item { return new(chunkstore.ChunkStampItem) },
			MarshalErr: postage.ErrInvalidBatchID,
		},
	}, {
		name: "min values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &chunkstore.ChunkStampItem{
				Address: minAddress,
				Stamp:   minStamp,
			},
			Factory: func() storage.Item { return &chunkstore.ChunkStampItem{Address: minAddress} },
			CmpOpts: []cmp.Option{cmp.AllowUnexported(postage.Stamp{})},
		},
	}, {
		name: "valid values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &chunkstore.ChunkStampItem{
				Address: chunk.Address(),
				Stamp:   chunk.Stamp(),
			},
			Factory: func() storage.Item { return &chunkstore.ChunkStampItem{Address: chunk.Address()} },
			CmpOpts: []cmp.Option{cmp.AllowUnexported(postage.Stamp{})},
		},
	}, {
		name: "invalid size",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &storagetest.ItemStub{
				MarshalBuf:   []byte{0xFF},
				UnmarshalBuf: []byte{0xFF},
			},
			Factory:      func() storage.Item { return &chunkstore.ChunkStampItem{Address: chunk.Address()} },
			UnmarshalErr: chunkstore.ErrUnmarshalInvalidChunkStampItemSize,
		},
	}}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			storagetest.TestItemMarshalAndUnmarshal(t, tc.test)
		})
	}
}

type memFS struct {
	afero.Fs
}

func (m *memFS) Open(path string) (fs.File, error) {
	return m.Fs.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
}

func TestChunkStore(t *testing.T) {
	t.Parallel()

	store := inmemstore.New()
	sharky, err := sharky.New(&memFS{Fs: afero.NewMemMapFs()}, 1, swarm.ChunkSize)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("inmem store close failed: %v", err)
		}
		if err := sharky.Close(); err != nil {
			t.Errorf("inmem sharky close failed: %v", err)
		}
	})

	st := chunkstore.New(store, sharky)

	testChunks := chunktest.GenerateTestRandomChunks(50)

	t.Run("put chunks", func(t *testing.T) {
		for _, ch := range testChunks {
			err := st.Put(context.TODO(), ch)
			if err != nil {
				t.Fatalf("failed putting new chunk: %v", err)
			}
		}
	})

	t.Run("put existing chunks", func(t *testing.T) {
		for idx, ch := range testChunks {
			// only put duplicates for odd numbered indexes
			if idx%2 != 0 {
				err := st.Put(context.TODO(), ch)
				if err != nil {
					t.Fatalf("failed putting new chunk: %v", err)
				}
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

	t.Run("delete unique chunks", func(t *testing.T) {
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

	// due to refCnt 2, these chunks should be present in the store even after delete
	t.Run("delete duplicate chunks", func(t *testing.T) {
		for idx, ch := range testChunks {
			if idx%2 != 0 {
				err := st.Delete(context.TODO(), ch.Address())
				if err != nil {
					t.Fatalf("failed deleting chunk: %v", err)
				}
			}
		}
	})

	t.Run("check chunks still exists", func(t *testing.T) {
		for idx, ch := range testChunks {
			if idx%2 != 0 {
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

	t.Run("delete duplicate chunks again", func(t *testing.T) {
		for idx, ch := range testChunks {
			if idx%2 != 0 {
				err := st.Delete(context.TODO(), ch.Address())
				if err != nil {
					t.Fatalf("failed deleting chunk: %v", err)
				}
			}
		}
	})

	t.Run("check all are deleted", func(t *testing.T) {
		count := 0
		err := st.Iterate(context.TODO(), func(_ swarm.Chunk) (bool, error) {
			count++
			return false, nil
		})
		if err != nil {
			t.Fatalf("unexpected error while iteration: %v", err)
		}
		if count != 0 {
			t.Fatalf("unexpected no of chunks, exp: %d, found: %d", 0, count)
		}
	})

	t.Run("close store", func(t *testing.T) {
		err := st.Close()
		if err != nil {
			t.Fatalf("unexpected error during close: %v", err)
		}
	})
}
