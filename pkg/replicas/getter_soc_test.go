// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package replicas_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/replicas"
	"github.com/ethersphere/bee/v2/pkg/replicas/combinator"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func TestSocGetter(t *testing.T) {
	t.Parallel()

	var (
		chunk     = swarm.NewChunk(swarm.NewAddress(make([]byte, 32)), make([]byte, 32))
		chunkAddr = chunk.Address()
		mock      = &mockGetter{
			getter: inmemchunkstore.New(),
		}
		socPutter = replicas.NewSocPutter(mock.getter, redundancy.MEDIUM)
		getter    = replicas.NewSocGetter(mock, redundancy.MEDIUM)
	)

	t.Run("happy path", func(t *testing.T) {
		if err := socPutter.Put(context.Background(), chunk); err != nil {
			t.Fatal(err)
		}
		got, err := getter.Get(context.Background(), chunkAddr)
		if err != nil {
			t.Fatalf("got error %v", err)
		}
		if !got.Equal(chunk) {
			t.Fatalf("got chunk %v, want %v", got, chunk)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := getter.Get(context.Background(), swarm.RandAddress(t))
		if !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("got error %v, want %v", err, storage.ErrNotFound)
		}
	})
}

func TestSocGetter_ReplicaFound(t *testing.T) {
	t.Parallel()

	var (
		chunk     = swarm.NewChunk(swarm.NewAddress(make([]byte, 32)), make([]byte, 32))
		chunkAddr = chunk.Address()
		mock      = &mockGetter{
			getter: inmemchunkstore.New(),
		}
		socPutter = replicas.NewSocPutter(mock.getter, redundancy.MEDIUM)
		getter    = replicas.NewSocGetter(mock, redundancy.MEDIUM)
	)

	var replicaChunk swarm.Chunk
	replicaIter := combinator.IterateReplicaAddresses(chunkAddr, int(redundancy.MEDIUM))
	for replicaAddr := range replicaIter {
		replicaChunk = swarm.NewChunk(replicaAddr, chunk.Data())
		if err := socPutter.Put(context.Background(), replicaChunk); err != nil {
			t.Fatal(err)
		}
		break
	}

	got, err := getter.Get(context.Background(), chunkAddr)
	if err != nil {
		t.Fatalf("got error %v", err)
	}
	if !got.Equal(chunk) {
		t.Fatalf("got chunk %v, want %v", got, chunk)
	}
}

func TestSocGetter_MultipleReplicasFound(t *testing.T) {
	t.Parallel()

	var (
		chunk     = swarm.NewChunk(swarm.NewAddress(make([]byte, 32)), make([]byte, 32))
		chunkAddr = chunk.Address()
		mock      = &mockGetter{
			getter: inmemchunkstore.New(),
		}
		socPutter = replicas.NewSocPutter(mock.getter, redundancy.MEDIUM)
		getter    = replicas.NewSocGetter(mock, redundancy.MEDIUM)
	)

	replicaIter := combinator.IterateReplicaAddresses(chunkAddr, int(redundancy.MEDIUM))
	var replicaChunk1, replicaChunk2 swarm.Chunk
	i := 0
	for replicaAddr := range replicaIter {
		if i == 0 {
			replicaChunk1 = swarm.NewChunk(replicaAddr, chunk.Data())
			if err := socPutter.Put(context.Background(), replicaChunk1); err != nil {
				t.Fatal(err)
			}
		} else {
			replicaChunk2 = swarm.NewChunk(replicaAddr, chunk.Data())
			if err := socPutter.Put(context.Background(), replicaChunk2); err != nil {
				t.Fatal(err)
			}
			break
		}
		i++
	}

	got, err := getter.Get(context.Background(), chunkAddr)
	if err != nil {
		t.Fatalf("got error %v", err)
	}

	if !got.Equal(chunk) {
		t.Fatalf("got unexpected chunk %v, want %v", got, chunk)
	}
}

func TestSocGetter_MaxRedundancyLevelLimit(t *testing.T) {
	t.Parallel()

	var (
		chunkAddr = swarm.RandAddress(t)
		mock      = &countingGetter{
			getter: inmemchunkstore.New(),
		}
		// Initialize SocGetter with a redundancy level higher than maxRedundancyLevel (which is 4)
		getter = replicas.NewSocGetter(mock, redundancy.Level(10))
	)

	// maxRedundancyLevel is 4, so 2^4 = 16 replicas. Total expected calls: 1 (original) + 16 (replicas) = 17.
	expectedCalls := 1 + (1 << 4) // 1 + 2^4 = 17

	_, err := getter.Get(context.Background(), chunkAddr)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if mock.calls != expectedCalls {
		t.Fatalf("expected %d Get calls, got %d", expectedCalls, mock.calls)
	}
}

func TestSocGetter_ContextCanceled(t *testing.T) {
	t.Parallel()

	var (
		chunkAddr = swarm.RandAddress(t)
		mock      = &mockGetterWithDelay{
			getter: inmemchunkstore.New(),
		}
		getter = replicas.NewSocGetter(mock, redundancy.MEDIUM)
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := getter.Get(ctx, chunkAddr)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got error %v, want %v", err, context.Canceled)
	}
}

func TestSocGetter_DeadlineExceeded(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		t.Helper()

		var (
			chunkAddr = swarm.RandAddress(t)
			mock      = &mockGetterWithDelay{
				getter:   inmemchunkstore.New(),
				getDelay: 100 * time.Millisecond,
			}
			getter = replicas.NewSocGetter(mock, redundancy.MEDIUM)
		)

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err := getter.Get(ctx, chunkAddr)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("got error %v, want %v", err, context.DeadlineExceeded)
		}
	})
}

func TestSocGetter_AllReplicasFail(t *testing.T) {
	t.Parallel()

	var (
		chunkAddr = swarm.RandAddress(t)
		mock      = &mockGetter{
			getter: inmemchunkstore.New(),
			err:    errors.New("some error"),
		}
		getter = replicas.NewSocGetter(mock, redundancy.MEDIUM)
	)

	_, err := getter.Get(context.Background(), chunkAddr)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSocGetter_PartialReplicaFailure(t *testing.T) {
	t.Parallel()

	var (
		chunk     = swarm.NewChunk(swarm.NewAddress(make([]byte, 32)), make([]byte, 32))
		chunkAddr = chunk.Address()
		mock      = &failingMockGetter{
			getter:    inmemchunkstore.New(),
			failAddrs: make(map[string]struct{}),
		}
		socPutter = replicas.NewSocPutter(mock.getter, redundancy.MEDIUM)
		getter    = replicas.NewSocGetter(mock, redundancy.MEDIUM)
	)

	replicaIter := combinator.IterateReplicaAddresses(chunkAddr, int(redundancy.MEDIUM))

	i := 0
	var successChunk swarm.Chunk
	for addr := range replicaIter {
		switch i {
		case 0:
			// First replica will fail
			mock.failAddrs[addr.String()] = struct{}{}
		case 1:
			// Second replica will succeed
			successChunk = swarm.NewChunk(addr, chunk.Data())
			if err := socPutter.Put(context.Background(), successChunk); err != nil {
				t.Fatal(err)
			}
		default:
			// Make other replicas fail
			mock.failAddrs[addr.String()] = struct{}{}
		}
		i++
	}

	got, err := getter.Get(context.Background(), chunkAddr)
	if err != nil {
		t.Fatalf("got error %v", err)
	}
	if !got.Equal(chunk) {
		t.Fatalf("got chunk %v, want %v", got, chunk)
	}
}

func TestSocGetter_DifferentRedundancyLevel(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                    string
		uploadRedundancyLevel   redundancy.Level
		retrieveRedundancyLevel redundancy.Level
	}{
		{
			name:                    "upload PARANOID, retrieve MEDIUM",
			uploadRedundancyLevel:   redundancy.PARANOID,
			retrieveRedundancyLevel: redundancy.MEDIUM,
		},
		{
			name:                    "upload PARANOID, retrieve STRONG",
			uploadRedundancyLevel:   redundancy.PARANOID,
			retrieveRedundancyLevel: redundancy.STRONG,
		},
		{
			name:                    "upload STRONG, retrieve MEDIUM",
			uploadRedundancyLevel:   redundancy.STRONG,
			retrieveRedundancyLevel: redundancy.MEDIUM,
		},
		{
			name:                    "upload MEDIUM, retrieve MEDIUM",
			uploadRedundancyLevel:   redundancy.MEDIUM,
			retrieveRedundancyLevel: redundancy.MEDIUM,
		},
		{
			name:                    "upload INSANE, retrieve MEDIUM",
			uploadRedundancyLevel:   redundancy.INSANE,
			retrieveRedundancyLevel: redundancy.MEDIUM,
		},
		{
			name:                    "upload INSANE, retrieve PARANOID",
			uploadRedundancyLevel:   redundancy.INSANE,
			retrieveRedundancyLevel: redundancy.PARANOID,
		},
		{
			name:                    "upload NONE, retrieve MEDIUM",
			uploadRedundancyLevel:   redundancy.NONE,
			retrieveRedundancyLevel: redundancy.MEDIUM,
		},
		{
			name:                    "upload MEDIUM, retrieve NONE",
			uploadRedundancyLevel:   redundancy.MEDIUM,
			retrieveRedundancyLevel: redundancy.NONE,
		},
		{
			name:                    "upload MEDIUM, retrieve STRONG (should still find if replica exists)",
			uploadRedundancyLevel:   redundancy.MEDIUM,
			retrieveRedundancyLevel: redundancy.STRONG,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var (
				chunk     = swarm.NewChunk(swarm.NewAddress(make([]byte, 32)), make([]byte, 32))
				chunkAddr = chunk.Address()
				mock      = &mockGetter{
					getter: inmemchunkstore.New(),
				}
			)

			// Use socPutter to put the original chunk and its replicas
			putter := replicas.NewSocPutter(mock.getter, tc.uploadRedundancyLevel)
			err := putter.Put(context.Background(), chunk)
			if err != nil {
				t.Fatalf("socPutter.Put failed: %v", err)
			}

			getter := replicas.NewSocGetter(mock, tc.retrieveRedundancyLevel)

			got, err := getter.Get(context.Background(), chunkAddr)
			if err != nil {
				t.Fatalf("got error %v", err)
			}
			if got == nil {
				t.Fatal("expected a chunk, got nil")
			}

			// Verify that the retrieved chunk is either the original or one of its replicas
			found := false
			if got.Equal(chunk) {
				found = true
			} else {
				replicaIter := combinator.IterateReplicaAddresses(chunkAddr, int(tc.uploadRedundancyLevel))
				for replicaAddr := range replicaIter {
					replicaChunk := swarm.NewChunk(replicaAddr, chunk.Data())
					if got.Equal(replicaChunk) {
						found = true
						break
					}
				}
			}

			if !found {
				t.Fatalf("retrieved chunk %v is neither the original nor any of its replicas", got)
			}
		})
	}
}

type mockGetter struct {
	getter storage.ChunkStore
	err    error
}

func (m *mockGetter) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.getter.Get(ctx, addr)
}

type failingMockGetter struct {
	getter    storage.ChunkStore
	failAddrs map[string]struct{}
	mu        sync.Mutex
}

func (m *failingMockGetter) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, found := m.failAddrs[addr.String()]; found {
		return nil, errors.New("failed to get chunk")
	}
	return m.getter.Get(ctx, addr)
}

type mockGetterWithDelay struct {
	getter   storage.ChunkStore
	err      error
	getDelay time.Duration
}

func (m *mockGetterWithDelay) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {
	if m.getDelay > 0 {
		time.Sleep(m.getDelay)
	}
	if m.err != nil {
		return nil, m.err
	}
	return m.getter.Get(ctx, addr)
}

// countingGetter is a mock storage.Getter that counts Get calls.

type countingGetter struct {
	getter storage.ChunkStore

	mu sync.Mutex

	calls int
}

func (c *countingGetter) Get(ctx context.Context, addr swarm.Address) (swarm.Chunk, error) {

	c.mu.Lock()

	c.calls++

	c.mu.Unlock()

	return c.getter.Get(ctx, addr)

}
