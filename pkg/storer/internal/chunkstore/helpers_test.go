// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ethersphere/bee/pkg/sharky"
	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/inmemstore"
	chunktest "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/storer/internal/chunkstore"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/spf13/afero"
)

// TestIterateLocations asserts that all stored chunks
// are retrievable by sharky using IterateLocations.
func TestIterateLocations(t *testing.T) {
	t.Parallel()

	const chunksCount = 50

	cs := makeChunkStore(t)
	testChunks := chunktest.GenerateTestRandomChunks(chunksCount)
	ctx := context.Background()

	for _, ch := range testChunks {
		assert.NoError(t, cs.chunkStore.Put(ctx, ch))
	}

	readCount := 0
	respC := make(chan chunkstore.LocationResult, chunksCount)
	chunkstore.IterateLocations(ctx, cs.store, respC)

	for resp := range respC {
		assert.NoError(t, resp.Err)

		buf := make([]byte, resp.Location.Length)
		assert.NoError(t, cs.sharky.Read(ctx, resp.Location, buf))

		assert.True(t, swarm.ContainsChunkWithData(testChunks, buf))
		readCount++
	}

	assert.Equal(t, chunksCount, readCount)
}

// TestIterateLocations_Stop asserts that IterateLocations will
// stop iteration when context is canceled.
func TestIterateLocations_Stop(t *testing.T) {
	t.Parallel()

	const chunksCount = 50
	const stopReadAt = 10

	cs := makeChunkStore(t)
	testChunks := chunktest.GenerateTestRandomChunks(chunksCount)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, ch := range testChunks {
		assert.NoError(t, cs.chunkStore.Put(ctx, ch))
	}

	readCount := 0
	respC := make(chan chunkstore.LocationResult)
	chunkstore.IterateLocations(ctx, cs.store, respC)

	for resp := range respC {
		if resp.Err != nil {
			assert.ErrorIs(t, resp.Err, context.Canceled)
			break
		}

		buf := make([]byte, resp.Location.Length)
		if err := cs.sharky.Read(ctx, resp.Location, buf); err != nil {
			assert.ErrorIs(t, err, context.Canceled)
			break
		}

		assert.True(t, swarm.ContainsChunkWithData(testChunks, buf))
		readCount++

		if readCount == stopReadAt {
			cancel()
		}
	}

	assert.InDelta(t, stopReadAt, readCount, 1)
}

type chunkStore struct {
	store      storage.Store
	sharky     *sharky.Store
	chunkStore storage.ChunkStore
}

func makeChunkStore(t *testing.T) *chunkStore {
	t.Helper()

	store := inmemstore.New()
	sharky, err := sharky.New(&memFS{Fs: afero.NewMemMapFs()}, 1, swarm.SocMaxChunkSize)
	assert.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, store.Close())
		assert.NoError(t, sharky.Close())
	})

	return &chunkStore{
		store:      store,
		sharky:     sharky,
		chunkStore: chunkstore.New(store, sharky),
	}
}
