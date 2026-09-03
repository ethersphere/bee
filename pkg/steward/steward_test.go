// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package steward_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/file/pipeline/builder"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/mock"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/steward"
	"github.com/ethersphere/bee/v2/pkg/storage"
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
		data           = make([]byte, chunks*4096) // 1k chunks
		chunkStore     = inmem
		store          = mockstorer.NewWithChunkStore(chunkStore)
		localRetrieval = &localRetriever{ChunkStore: chunkStore}
		s              = steward.New(store, localRetrieval, inmem)
		stamper        = postagetesting.NewStamper()
	)
	n, err := rand.Read(data)
	if n != cap(data) {
		t.Fatal("short read")
	}
	if err != nil {
		t.Fatal(err)
	}

	pipe := builder.NewPipelineBuilder(ctx, chunkStore, false, redundancy.NONE)
	addr, err := builder.FeedPipeline(ctx, pipe, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	chunkCount := int(inmem.count.Load())
	replicaCount := redundancy.PARANOID.GetReplicaCount()
	wantPushed := chunkCount + replicaCount
	done := make(chan struct{})
	errc := make(chan error, 1)
	replicaAddrs := make(map[string]struct{})
	var replicaMu sync.Mutex
	go func() {
		defer close(done)
		count := 0
		for op := range store.PusherFeed() {
			// DirectUpload only forwards pushed chunks over the feed; it does not
			// persist them. Persist here so the post-reupload assertions (Has,
			// IsRetrievable) observe pushed-but-not-yet-locally-known chunks the
			// same way a real pushsync round-trip eventually would.
			if err := chunkStore.Put(ctx, op.Chunk); err != nil {
				select {
				case errc <- err:
				default:
				}
				return
			}

			if sch, err := soc.FromChunk(op.Chunk); err == nil && bytes.Equal(sch.OwnerAddress(), swarm.ReplicasOwner) {
				replicaMu.Lock()
				replicaAddrs[op.Chunk.Address().String()] = struct{}{}
				replicaMu.Unlock()
			}

			count++
			if count == wantPushed {
				return
			}
		}
	}()

	err = s.Reupload(ctx, addr, stamper, redundancy.PARANOID)
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

	isRetrievable, err := s.IsRetrievable(ctx, addr, redundancy.PARANOID)
	if err != nil {
		t.Fatal(err)
	}
	if !isRetrievable {
		t.Fatalf("re-uploaded content on %q should be retrievable", addr)
	}

	count := len(localRetrieval.retrievedChunks)
	// IsRetrievable's root-chunk fetch goes through joiner -> replicas.NewGetter, which
	// races the original root address against an initial batch of 2 replica candidate
	// addresses before the first success cancels the rest (see replicas/getter.go). With
	// real dispersed replicas now present (this is what this fix creates), up to 2 of
	// those speculative replica fetches can also succeed and get recorded before
	// cancellation lands, on top of the trie chunks retrieved by traversal.
	const maxSpeculativeRootFetches = 2
	if count < chunkCount || count > chunkCount+maxSpeculativeRootFetches {
		t.Fatalf("unexpected no of unique chunks retrieved: want between %d and %d, have %d", chunkCount, chunkCount+maxSpeculativeRootFetches, count)
	}

	replicaMu.Lock()
	gotReplicas := len(replicaAddrs)
	replicaMu.Unlock()
	if gotReplicas != replicaCount {
		t.Fatalf("unexpected no of dispersed replicas re-uploaded: want %d have %d", replicaCount, gotReplicas)
	}
}

type localRetriever struct {
	storage.ChunkStore
	mu              sync.Mutex
	retrievedChunks map[string]struct{}
}

func (lr *localRetriever) RetrieveChunk(ctx context.Context, addr, sourceAddr swarm.Address) (chunk swarm.Chunk, err error) {
	ch, err := lr.Get(ctx, addr)
	if err != nil {
		return nil, err
	}

	lr.mu.Lock()
	defer lr.mu.Unlock()

	if lr.retrievedChunks == nil {
		lr.retrievedChunks = make(map[string]struct{})
	}
	lr.retrievedChunks[addr.String()] = struct{}{}
	return ch, nil
}
