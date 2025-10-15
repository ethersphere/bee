// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"context"
	"testing"
	"time"

	chunktest "github.com/ethersphere/bee/v2/pkg/storage/testing"
	storer "github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/google/go-cmp/cmp"
)

func emptyBinIDs() []uint64 {
	return make([]uint64, swarm.MaxBins)
}

func testDebugInfo(t *testing.T, newStorer func() (*storer.DB, swarm.Address, error)) {
	t.Helper()

	t.Run("upload and pin", func(t *testing.T) {
		t.Parallel()

		lstore, _, err := newStorer()
		if err != nil {
			t.Fatal(err)
		}

		_, epoch, err := lstore.ReserveLastBinIDs()
		if err != nil {
			t.Fatal(err)
		}

		tag, err := lstore.NewSession()
		if err != nil {
			t.Fatalf("NewSession(): unexpected error: %v", err)
		}

		session, err := lstore.Upload(context.Background(), true, tag.TagID)
		if err != nil {
			t.Fatalf("Upload(...): unexpected error: %v", err)
		}

		chunks := chunktest.GenerateTestRandomChunks(10)
		for _, ch := range chunks {
			err := session.Put(context.Background(), ch)
			if err != nil {
				t.Fatalf("session.Put(...): unexpected error: %v", err)
			}
		}

		err = session.Done(chunks[0].Address())
		if err != nil {
			t.Fatalf("session.Done(...): unexpected error: %v", err)
		}

		info, err := lstore.DebugInfo(context.Background())
		if err != nil {
			t.Fatalf("DebugInfo(...): unexpected error: %v", err)
		}

		// Because the chunks in the session where never 'Reported' as synced, the pending upload will be non-zero.

		wantInfo := storer.Info{
			Upload: storer.UploadStat{
				TotalUploaded: 10,
				TotalSynced:   0,
				PendingUpload: 10,
			},
			Pinning: storer.PinningStat{
				TotalCollections: 1,
				TotalChunks:      10,
			},
			ChunkStore: storer.ChunkStoreStat{
				TotalChunks:    10,
				SharedSlots:    10,
				ReferenceCount: 20,
			},
			Cache: storer.CacheStat{
				Capacity: 1000000,
			},
			Reserve: storer.ReserveStat{
				Capacity:   100,
				LastBinIDs: emptyBinIDs(),
				Epoch:      epoch,
			},
		}

		if diff := cmp.Diff(wantInfo, info); diff != "" {
			t.Fatalf("invalid info (+want -have):\n%s", diff)
		}
	})

	t.Run("cache", func(t *testing.T) {
		t.Parallel()

		lstore, _, err := newStorer()
		if err != nil {
			t.Fatal(err)
		}

		_, epoch, err := lstore.ReserveLastBinIDs()
		if err != nil {
			t.Fatal(err)
		}

		chunks := chunktest.GenerateTestRandomChunks(10)
		for _, ch := range chunks {
			err := lstore.Cache().Put(context.Background(), ch)
			if err != nil {
				t.Fatalf("session.Put(...): unexpected error: %v", err)
			}
		}

		info, err := lstore.DebugInfo(context.Background())
		if err != nil {
			t.Fatalf("DebugInfo(...): unexpected error: %v", err)
		}

		wantInfo := storer.Info{
			ChunkStore: storer.ChunkStoreStat{
				TotalChunks:    10,
				ReferenceCount: 10,
			},
			Cache: storer.CacheStat{
				Size:     10,
				Capacity: 1000000,
			},
			Reserve: storer.ReserveStat{
				Capacity:   100,
				LastBinIDs: emptyBinIDs(),
				Epoch:      epoch,
			},
		}

		if diff := cmp.Diff(wantInfo, info); diff != "" {
			t.Fatalf("invalid info (+want -have):\n%s", diff)
		}
	})

	t.Run("reserve", func(t *testing.T) {
		t.Parallel()

		lstore, addr, err := newStorer()
		if err != nil {
			t.Fatal(err)
		}

		_, epoch, err := lstore.ReserveLastBinIDs()
		if err != nil {
			t.Fatal(err)
		}

		putter := lstore.ReservePutter()

		for i := 0; i < 10; i++ {
			chunk := chunktest.GenerateTestRandomChunkAt(t, addr, 0)
			err := putter.Put(context.Background(), chunk)
			if err != nil {
				t.Fatalf("session.Put(...): unexpected error: %v", err)
			}
		}

		info, err := lstore.DebugInfo(context.Background())
		if err != nil {
			t.Fatalf("DebugInfo(...): unexpected error: %v", err)
		}

		ids := emptyBinIDs()
		ids[0] = 10

		wantInfo := storer.Info{
			ChunkStore: storer.ChunkStoreStat{
				TotalChunks:    10,
				ReferenceCount: 10,
			},
			Cache: storer.CacheStat{
				Capacity: 1000000,
			},
			Reserve: storer.ReserveStat{
				SizeWithinRadius: 10,
				TotalSize:        10,
				Capacity:         100,
				LastBinIDs:       ids,
				Epoch:            epoch,
			},
		}

		if diff := cmp.Diff(wantInfo, info); diff != "" {
			t.Fatalf("invalid info (+want -have):\n%s", diff)
		}
	})

}

func TestDebugInfo(t *testing.T) {
	t.Parallel()

	t.Run("inmem", func(t *testing.T) {
		t.Parallel()

		testDebugInfo(t, func() (*storer.DB, swarm.Address, error) {
			addr := swarm.RandAddress(t)
			store, err := storer.New(context.Background(), "", dbTestOps(addr, 100, nil, nil, time.Second))
			return store, addr, err
		})
	})
	t.Run("disk", func(t *testing.T) {
		t.Parallel()

		testDebugInfo(t, func() (*storer.DB, swarm.Address, error) {
			addr := swarm.RandAddress(t)
			store, err := diskStorer(t, dbTestOps(addr, 100, nil, nil, time.Second))()
			return store, addr, err
		})
	})
}
