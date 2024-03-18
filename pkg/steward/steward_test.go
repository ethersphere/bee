// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package steward_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/mock"
	"github.com/ethersphere/bee/v2/pkg/steward"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemchunkstore"
	mockstorer "github.com/ethersphere/bee/v2/pkg/storer/mock"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type counter struct {
	storage.ChunkStore
	count atomic.Int32
}

func (c *counter) Put(ctx context.Context, ch swarm.Chunk) (err error) {
	c.count.Add(1)
	return c.ChunkStore.Put(ctx, ch)
}

func TestSteward(t *testing.T) {
	t.Parallel()
	inmem := &counter{ChunkStore: inmemchunkstore.New()}

	var (
		ctx            = context.Background()
		chunks         = 1000
		data           = make([]byte, chunks*4096) //1k chunks
		chunkStore     = inmem
		store          = mockstorer.NewWithChunkStore(chunkStore)
		localRetrieval = &localRetriever{ChunkStore: chunkStore}
		s              = steward.New(store, localRetrieval, inmem)
		stamper        = postagetesting.NewStamper()
	)
	ctx = redundancy.SetLevelInContext(ctx, redundancy.NONE)

	n, err := rand.Read(data)
	if n != cap(data) {
		t.Fatal("short read")
	}
	if err != nil {
		t.Fatal(err)
	}

	pipe := builder.NewPipelineBuilder(ctx, chunkStore, false, 0)
	addr, err := builder.FeedPipeline(ctx, pipe, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	chunkCount := int(inmem.count.Load())
	done := make(chan struct{})
	errc := make(chan error, 1)
	go func() {
		defer close(done)
		count := 0
		for op := range store.PusherFeed() {
			has, err := chunkStore.Has(ctx, op.Chunk.Address())
			if err != nil || !has {
				if !has {
					err = errors.New("chunk not found")
				}
				select {
				case errc <- err:
				default:
				}
				return
			}
			count++
			if count == chunkCount {
				return
			}
		}
	}()

	err = s.Reupload(ctx, addr, stamper)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("took too long to finish")
	}

	select {
	case err := <-errc:
		t.Fatalf("unexpected error: %v", err)
	default:
	}

	isRetrievable, err := s.IsRetrievable(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	if !isRetrievable {
		t.Fatalf("re-uploaded content on %q should be retrievable", addr)
	}

	count := len(localRetrieval.retrievedChunks)
	if count != chunkCount {
		t.Fatalf("unexpected no of unique chunks retrieved: want %d have %d", chunkCount, count)
	}
}

type localRetriever struct {
	storage.ChunkStore
	mu              sync.Mutex
	retrievedChunks map[string]struct{}
}

func (lr *localRetriever) RetrieveChunk(ctx context.Context, addr, sourceAddr swarm.Address) (chunk swarm.Chunk, err error) {
	lr.mu.Lock()
	defer lr.mu.Unlock()

	if lr.retrievedChunks == nil {
		lr.retrievedChunks = make(map[string]struct{})
	}
	lr.retrievedChunks[addr.String()] = struct{}{}
	return lr.Get(ctx, addr)
}
