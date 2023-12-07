// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"context"
	"os"
	"path"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/postage"
	batchstore "github.com/ethersphere/bee/pkg/postage/batchstore/mock"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/inmemchunkstore"
	"github.com/ethersphere/bee/pkg/storage/migration"
	"github.com/ethersphere/bee/pkg/storer"
	pinstore "github.com/ethersphere/bee/pkg/storer/internal/pinning"
	"github.com/ethersphere/bee/pkg/storer/internal/upload"
	localmigration "github.com/ethersphere/bee/pkg/storer/migration"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/topology"
	kademlia "github.com/ethersphere/bee/pkg/topology/mock"
)

func verifyChunks(
	t *testing.T,
	repo storage.Repository,
	chunks []swarm.Chunk,
	has bool,
) {
	t.Helper()

	for _, ch := range chunks {
		hasFound, err := repo.ChunkStore().Has(context.TODO(), ch.Address())
		if err != nil {
			t.Fatalf("ChunkStore.Has(...): unexpected error: %v", err)
		}

		if hasFound != has {
			t.Fatalf("unexpected chunk has state: want %t have %t", has, hasFound)
		}
	}

}

func verifySessionInfo(
	t *testing.T,
	repo storage.Repository,
	sessionID uint64,
	chunks []swarm.Chunk,
	has bool,
) {
	t.Helper()

	verifyChunks(t, repo, chunks, has)

	if has {
		tagInfo, err := upload.TagInfo(repo.IndexStore(), sessionID)
		if err != nil {
			t.Fatalf("upload.TagInfo(...): unexpected error: %v", err)
		}

		if tagInfo.Split != uint64(len(chunks)) {
			t.Fatalf("unexpected split chunk count in tag: want %d have %d", len(chunks), tagInfo.Split)
		}
		if tagInfo.Seen != 0 {
			t.Fatalf("unexpected seen chunk count in tag: want %d have %d", len(chunks), tagInfo.Seen)
		}
	}
}

func verifyPinCollection(
	t *testing.T,
	repo storage.Repository,
	root swarm.Chunk,
	chunks []swarm.Chunk,
	has bool,
) {
	t.Helper()

	hasFound, err := pinstore.HasPin(repo.IndexStore(), root.Address())
	if err != nil {
		t.Fatalf("pinstore.HasPin(...): unexpected error: %v", err)
	}

	if hasFound != has {
		t.Fatalf("unexpected pin collection state: want %t have %t", has, hasFound)
	}

	verifyChunks(t, repo, chunks, has)
}

// TestMain exists to adjust the time.Now function to a fixed value.
func TestMain(m *testing.M) {
	storer.ReplaceSharkyShardLimit(4)
	defer func() {
		storer.ReplaceSharkyShardLimit(32)
	}()
	code := m.Run()
	os.Exit(code)
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("inmem with options", func(t *testing.T) {
		t.Parallel()

		opts := dbTestOps(swarm.RandAddress(t), 0, nil, nil, time.Second)

		lstore := makeInmemStorer(t, opts)
		if lstore == nil {
			t.Fatalf("storer should be instantiated")
		}
	})
	t.Run("disk default options", func(t *testing.T) {
		t.Parallel()

		lstore := makeDiskStorer(t, dbTestOps(swarm.RandAddress(t), 0, nil, nil, time.Second))
		if lstore == nil {
			t.Fatalf("storer should be instantiated")
		}
	})
	t.Run("disk with options", func(t *testing.T) {
		t.Parallel()

		opts := dbTestOps(swarm.RandAddress(t), 0, nil, nil, time.Second)
		opts.CacheCapacity = 10

		lstore := makeDiskStorer(t, opts)
		if lstore == nil {
			t.Fatalf("storer should be instantiated")
		}
	})

	t.Run("migration on latest version", func(t *testing.T) {
		t.Parallel()

		t.Run("inmem", func(t *testing.T) {
			t.Parallel()

			lstore := makeInmemStorer(t, dbTestOps(swarm.RandAddress(t), 0, nil, nil, time.Second))
			assertStorerVersion(t, lstore, "")
		})

		t.Run("disk", func(t *testing.T) {
			t.Parallel()

			lstore := makeDiskStorer(t, dbTestOps(swarm.RandAddress(t), 0, nil, nil, time.Second))
			assertStorerVersion(t, lstore, path.Join(t.TempDir(), "sharky"))
		})
	})
}

func dbTestOps(baseAddr swarm.Address, reserveCapacity int, bs postage.Storer, radiusSetter topology.SetStorageRadiuser, reserveWakeUpTime time.Duration) *storer.Options {

	opts := storer.DefaultOptions()

	if radiusSetter == nil {
		radiusSetter = kademlia.NewTopologyDriver()
	}

	if bs == nil {
		bs = batchstore.New()
	}

	opts.Address = baseAddr
	opts.RadiusSetter = radiusSetter
	opts.ReserveCapacity = reserveCapacity
	opts.ReserveMinimumRadius = 0
	opts.Batchstore = bs
	opts.ReserveWakeUpDuration = reserveWakeUpTime
	opts.Logger = log.Noop

	return opts
}

func assertStorerVersion(t *testing.T, lstore *storer.DB, sharkyPath string) {
	t.Helper()

	current, err := migration.Version(lstore.Repo().IndexStore(), "migration")
	if err != nil {
		t.Fatalf("migration.Version(...): unexpected error: %v", err)
	}

	expected := migration.LatestVersion(localmigration.AfterInitSteps(sharkyPath, 4, inmemchunkstore.New()))

	if current != expected {
		t.Fatalf("storer is not migrated to latest version; got %d, expected %d", current, expected)
	}
}

func makeInmemStorer(t *testing.T, opts *storer.Options) *storer.DB {
	t.Helper()

	lstore, err := storer.New(context.Background(), "", opts)
	if err != nil {
		t.Fatalf("New(...): unexpected error: %v", err)
	}

	t.Cleanup(func() {
		err := lstore.Close()
		if err != nil {
			t.Fatalf("Close(): unexpected error: %v", err)
		}
	})

	return lstore
}

func makeDiskStorer(t *testing.T, opts *storer.Options) *storer.DB {
	t.Helper()

	lstore, err := storer.New(context.Background(), t.TempDir(), opts)
	if err != nil {
		t.Fatalf("New(...): unexpected error: %v", err)
	}

	t.Cleanup(func() {
		err := lstore.Close()
		if err != nil {
			t.Fatalf("Close(): unexpected closing storer: %v", err)
		}
	})

	return lstore
}

func newStorer(tb testing.TB, path string, opts *storer.Options) (*storer.DB, error) {
	tb.Helper()
	lstore, err := storer.New(context.Background(), path, opts)
	if err == nil {
		tb.Cleanup(func() {
			err := lstore.Close()
			if err != nil {
				tb.Errorf("failed closing storer: %v", err)
			}
		})
	}

	return lstore, err
}

func diskStorer(tb testing.TB, opts *storer.Options) func() (*storer.DB, error) {
	tb.Helper()
	return func() (*storer.DB, error) {
		return newStorer(tb, tb.TempDir(), opts)
	}
}

func memStorer(tb testing.TB, opts *storer.Options) func() (*storer.DB, error) {
	tb.Helper()
	return func() (*storer.DB, error) {
		return newStorer(tb, "", opts)
	}
}
