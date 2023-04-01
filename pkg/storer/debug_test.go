// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"context"
	"testing"
	"time"

	chunktest "github.com/ethersphere/bee/pkg/storage/testing"
	storer "github.com/ethersphere/bee/pkg/storer"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/google/go-cmp/cmp"
)

func testDebugInfo(t *testing.T, newStorer func() (*storer.DB, error)) {
	t.Helper()

	t.Run("upload and pin", func(t *testing.T) {
		t.Parallel()

		lstore, err := newStorer()
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

		wantInfo := storer.Info{
			Upload: storer.UploadStat{
				TotalUploaded: 10,
				TotalSynced:   0,
			},
			Pinning: storer.PinningStat{
				TotalCollections: 1,
				TotalChunks:      10,
			},
			ChunkStore: storer.ChunkStoreStat{
				TotalChunks: 10,
				SharedSlots: 10,
			},
			Cache: storer.CacheStat{
				Capacity: 1000000,
			},
			Reserve: storer.ReserveStat{
				Capacity: 100,
			},
		}

		if diff := cmp.Diff(wantInfo, info); diff != "" {
			t.Fatalf("invalid info (+want -have):\n%s", diff)
		}
	})

	t.Run("cache", func(t *testing.T) {
		t.Parallel()

		lstore, err := newStorer()
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
				TotalChunks: 10,
			},
			Cache: storer.CacheStat{
				Size:     10,
				Capacity: 1000000,
			},
			Reserve: storer.ReserveStat{
				Capacity: 100,
			},
		}

		if diff := cmp.Diff(wantInfo, info); diff != "" {
			t.Fatalf("invalid info (+want -have):\n%s", diff)
		}
	})

	t.Run("reserve", func(t *testing.T) {
		t.Parallel()

		lstore, err := newStorer()
		if err != nil {
			t.Fatal(err)
		}

		putter := lstore.ReservePutter(context.Background())

		chunks := chunktest.GenerateTestRandomChunks(10)
		for _, ch := range chunks {
			err := putter.Put(context.Background(), ch)
			if err != nil {
				t.Fatalf("session.Put(...): unexpected error: %v", err)
			}
		}

		err = putter.Done(swarm.ZeroAddress)
		if err != nil {
			t.Fatalf("session.Done(...): unexpected error: %v", err)
		}

		info, err := lstore.DebugInfo(context.Background())
		if err != nil {
			t.Fatalf("DebugInfo(...): unexpected error: %v", err)
		}

		wantInfo := storer.Info{
			ChunkStore: storer.ChunkStoreStat{
				TotalChunks: 10,
			},
			Cache: storer.CacheStat{
				Capacity: 1000000,
			},
			Reserve: storer.ReserveStat{
				Size:     10,
				Capacity: 100,
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

		testDebugInfo(t, func() (*storer.DB, error) {
			return storer.New(context.Background(), "", dbTestOps(swarm.RandAddress(t), 100, nil, nil, time.Second))
		})
	})
	t.Run("disk", func(t *testing.T) {
		t.Parallel()

		testDebugInfo(t, diskStorer(t, dbTestOps(swarm.RandAddress(t), 100, nil, nil, time.Second)))
	})
}
