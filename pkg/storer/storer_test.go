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

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	batchstore "github.com/ethersphere/bee/v2/pkg/postage/batchstore/mock"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/migration"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	cs "github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
	pinstore "github.com/ethersphere/bee/v2/pkg/storer/internal/pinning"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/upload"
	localmigration "github.com/ethersphere/bee/v2/pkg/storer/migration"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/topology"
	kademlia "github.com/ethersphere/bee/v2/pkg/topology/mock"
)

func verifyChunks(
	t *testing.T,
	st transaction.Storage,
	chunks []swarm.Chunk,
	has bool,
) {
	t.Helper()

	for _, ch := range chunks {
		hasFound, err := st.ChunkStore().Has(context.TODO(), ch.Address())
		if err != nil {
			t.Fatalf("ChunkStore.Has(...): unexpected error: %v", err)
		}

		if hasFound != has {
			t.Fatalf("unexpected chunk has state: want %t have %t", has, hasFound)
		}
	}
}

func verifyChunkRefCount(
	t *testing.T,
	st transaction.ReadOnlyStore,
	chunks []swarm.Chunk,
) {
	t.Helper()

	for _, ch := range chunks {
		_ = st.IndexStore().Iterate(storage.Query{
			Factory: func() storage.Item { return new(cs.RetrievalIndexItem) },
		}, func(r storage.Result) (bool, error) {
			entry := r.Entry.(*cs.RetrievalIndexItem)
			if entry.Address.Equal(ch.Address()) && entry.RefCnt != 1 {
				t.Errorf("chunk %s has refCnt=%d", ch.Address(), entry.RefCnt)
			}
			return false, nil
		})
	}
}

func verifySessionInfo(
	t *testing.T,
	st transaction.Storage,
	sessionID uint64,
	chunks []swarm.Chunk,
	has bool,
) {
	t.Helper()

	verifyChunks(t, st, chunks, has)

	if has {
		tagInfo, err := upload.TagInfo(st.IndexStore(), sessionID)
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
	st transaction.Storage,
	root swarm.Chunk,
	chunks []swarm.Chunk,
	has bool,
) {
	t.Helper()

	hasFound, err := pinstore.HasPin(st.IndexStore(), root.Address())
	if err != nil {
		t.Fatalf("pinstore.HasPin(...): unexpected error: %v", err)
	}

	if hasFound != has {
		t.Fatalf("unexpected pin collection state: want %t have %t", has, hasFound)
	}

	verifyChunks(t, st, chunks, has)
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
			assertStorerVersion(t, lstore.Storage().IndexStore(), "")
		})

		t.Run("disk", func(t *testing.T) {
			t.Parallel()

			lstore := makeDiskStorer(t, dbTestOps(swarm.RandAddress(t), 0, nil, nil, time.Second))
			assertStorerVersion(t, lstore.Storage().IndexStore(), path.Join(t.TempDir(), "sharky"))
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
	opts.Batchstore = bs
	opts.ReserveWakeUpDuration = reserveWakeUpTime

	return opts
}

func assertStorerVersion(t *testing.T, r storage.Reader, sharkyPath string) {
	t.Helper()

	current, err := migration.Version(r, "migration")
	if err != nil {
		t.Fatalf("migration.Version(...): unexpected error: %v", err)
	}

	expected := migration.LatestVersion(localmigration.AfterInitSteps(sharkyPath, 4, internal.NewInmemStorage(), log.Noop))
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
