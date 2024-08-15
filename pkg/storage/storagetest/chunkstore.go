// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storagetest

import (
	"context"
	"errors"
	"testing"

	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	chunktest "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// TestChunkStore runs a correctness test suite on a given ChunkStore.
func TestChunkStore(t *testing.T, st storage.ChunkStore) {
	t.Helper()

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
		for _, ch := range testChunks {
			err := st.Put(context.TODO(), ch)
			if err != nil {
				t.Fatalf("failed putting new chunk: %v", err)
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
				t.Fatal("read chunk doesn't match")
			}
		}
	})

	t.Run("get non-existing chunk", func(t *testing.T) {
		stamp := postagetesting.MustNewStamp()
		ch := chunktest.GenerateTestRandomChunk().WithStamp(stamp)

		_, err := st.Get(context.TODO(), ch.Address())
		if !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("expected error %v", storage.ErrNotFound)
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
				_, err = st.Get(context.TODO(), ch.Address())
				if err != nil {
					t.Fatalf("expected no error, found: %v", err)
				}
				// delete twice as it was put twice
				err = st.Delete(context.TODO(), ch.Address())
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
					t.Fatal("read chunk doesn't match")
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
}

func RunChunkStoreBenchmarkTests(b *testing.B, s storage.ChunkStore) {
	b.Helper()

	b.Run("WriteSequential", func(b *testing.B) {
		BenchmarkChunkStoreWriteSequential(b, s)
	})
	b.Run("WriteRandom", func(b *testing.B) {
		BenchmarkChunkStoreWriteRandom(b, s)
	})
	b.Run("ReadSequential", func(b *testing.B) {
		BenchmarkChunkStoreReadSequential(b, s)
	})
	b.Run("ReadRandom", func(b *testing.B) {
		BenchmarkChunkStoreReadRandom(b, s)
	})
	b.Run("ReadRandomMissing", func(b *testing.B) {
		BenchmarkChunkStoreReadRandomMissing(b, s)
	})
	b.Run("ReadReverse", func(b *testing.B) {
		BenchmarkChunkStoreReadReverse(b, s)
	})
	b.Run("ReadRedHot", func(b *testing.B) {
		BenchmarkChunkStoreReadHot(b, s)
	})
	b.Run("IterateSequential", func(b *testing.B) {
		BenchmarkChunkStoreIterateSequential(b, s)
	})
	b.Run("IterateReverse", func(b *testing.B) {
		BenchmarkChunkStoreIterateReverse(b, s)
	})
	b.Run("DeleteRandom", func(b *testing.B) {
		BenchmarkChunkStoreDeleteRandom(b, s)
	})
	b.Run("DeleteSequential", func(b *testing.B) {
		BenchmarkChunkStoreDeleteSequential(b, s)
	})
}

func BenchmarkChunkStoreWriteSequential(b *testing.B, s storage.Putter) {
	b.Helper()

	doWriteChunk(b, s, newSequentialEntryGenerator(b.N))
}

func BenchmarkChunkStoreWriteRandom(b *testing.B, s storage.Putter) {
	b.Helper()

	doWriteChunk(b, s, newFullRandomEntryGenerator(0, b.N))
}

func BenchmarkChunkStoreReadSequential(b *testing.B, s storage.ChunkStore) {
	b.Helper()

	g := newSequentialKeyGenerator(b.N)
	doWriteChunk(b, s, newFullRandomEntryGenerator(0, b.N))
	resetBenchmark(b)
	doReadChunk(b, s, g, false)
}

func BenchmarkChunkStoreReadRandom(b *testing.B, s storage.ChunkStore) {
	b.Helper()

	g := newRandomKeyGenerator(b.N)
	doWriteChunk(b, s, newFullRandomEntryGenerator(0, b.N))
	resetBenchmark(b)
	doReadChunk(b, s, g, false)
}

func BenchmarkChunkStoreReadRandomMissing(b *testing.B, s storage.ChunkStore) {
	b.Helper()

	g := newRandomMissingKeyGenerator(b.N)
	resetBenchmark(b)
	doReadChunk(b, s, g, true)
}

func BenchmarkChunkStoreReadReverse(b *testing.B, db storage.ChunkStore) {
	b.Helper()

	g := newReversedKeyGenerator(newSequentialKeyGenerator(b.N))
	doWriteChunk(b, db, newFullRandomEntryGenerator(0, b.N))
	resetBenchmark(b)
	doReadChunk(b, db, g, false)
}

func BenchmarkChunkStoreReadHot(b *testing.B, s storage.ChunkStore) {
	b.Helper()

	k := maxInt((b.N+99)/100, 1)
	g := newRoundKeyGenerator(newRandomKeyGenerator(k))
	doWriteChunk(b, s, newFullRandomEntryGenerator(0, b.N))
	resetBenchmark(b)
	doReadChunk(b, s, g, false)
}

func BenchmarkChunkStoreIterateSequential(b *testing.B, s storage.ChunkStore) {
	b.Helper()

	var counter int
	_ = s.Iterate(context.Background(), func(c swarm.Chunk) (stop bool, err error) {
		counter++
		if counter > b.N {
			return true, nil
		}
		return false, nil
	})
}

func BenchmarkChunkStoreIterateReverse(b *testing.B, s storage.ChunkStore) {
	b.Helper()

	b.Skip("not implemented")
}

func BenchmarkChunkStoreDeleteRandom(b *testing.B, s storage.ChunkStore) {
	b.Helper()

	doDeleteChunk(b, s, newFullRandomEntryGenerator(0, b.N))
}

func BenchmarkChunkStoreDeleteSequential(b *testing.B, s storage.ChunkStore) {
	b.Helper()

	doDeleteChunk(b, s, newSequentialEntryGenerator(b.N))
}
