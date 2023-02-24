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
	"testing"

	"github.com/ethersphere/bee/pkg/file/pipeline/builder"
	mockstorer "github.com/ethersphere/bee/pkg/localstorev2/mock"
	"github.com/ethersphere/bee/pkg/steward"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/inmemchunkstore"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestSteward(t *testing.T) {
	t.Parallel()

	var (
		ctx            = context.Background()
		chunks         = 1000
		data           = make([]byte, chunks*4096) //1k chunks
		chunkStore     = inmemchunkstore.New()
		store          = mockstorer.NewWithChunkStore(chunkStore)
		localRetrieval = &localRetriever{ChunkStore: chunkStore}
		s              = steward.New(store, localRetrieval)
	)

	n, err := rand.Read(data)
	if n != cap(data) {
		t.Fatal("short read")
	}
	if err != nil {
		t.Fatal(err)
	}

	pipe := builder.NewPipelineBuilder(ctx, chunkStore, false)
	addr, err := builder.FeedPipeline(ctx, pipe, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	chunkCount := 0
	err = chunkStore.Iterate(context.Background(), func(ch swarm.Chunk) (bool, error) {
		chunkCount++
		return false, nil
	})

	count := 0
	stop := make(chan struct{})
	errc := make(chan error, 1)
	go func() {
		for {
			select {
			case <-stop:
				return
			case op := <-store.PusherFeed():
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
			}
		}
	}()

	err = s.Reupload(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-errc:
		t.Fatalf("unexpected error: %v", err)
	default:
	}

	if count != chunkCount {
		t.Fatalf("unexpected no of chunks pushed: want %d have %d", chunkCount, count)
	}

	isRetrievable, err := s.IsRetrievable(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	if !isRetrievable {
		t.Fatalf("re-uploaded content on %q should be retrievable", addr)
	}

	count = len(localRetrieval.retrievedChunks)
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
