// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	storer "github.com/ethersphere/bee/pkg/localstorev2"
	pinstore "github.com/ethersphere/bee/pkg/localstorev2/internal/pinning"
	"github.com/ethersphere/bee/pkg/localstorev2/internal/upload"
	localmigration "github.com/ethersphere/bee/pkg/localstorev2/migration"
	"github.com/ethersphere/bee/pkg/log"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/migration"
	"github.com/ethersphere/bee/pkg/swarm"
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
		tagInfo, err := upload.GetTagInfo(repo.IndexStore(), sessionID)
		if err != nil {
			t.Fatalf("upload.GetTagInfo(...): unexpected error: %v", err)
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
	code := m.Run()
	storer.ReplaceSharkyShardLimit(32)
	os.Exit(code)
}

func diskStorer(t *testing.T, opts *storer.Options) func() (*storer.DB, error) {
	t.Helper()

	return func() (*storer.DB, error) {
		dir, err := ioutil.TempDir(".", "testrepo*")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			err := os.RemoveAll(dir)
			if err != nil {
				t.Errorf("failed removing directories: %v", err)
			}
		})

		lstore, err := storer.New(dir, opts)
		if err == nil {
			t.Cleanup(func() {
				err := lstore.Close()
				if err != nil {
					t.Errorf("failed closing storer: %v", err)
				}
			})
		}

		return lstore, err
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("inmem default options", func(t *testing.T) {
		t.Parallel()

		lstore := makeInmemStorer(t, nil)
		if lstore == nil {
			t.Fatalf("storer should be instantiated")
		}
	})
	t.Run("inmem with options", func(t *testing.T) {
		t.Parallel()

		opts := &storer.Options{
			Logger: log.Noop,
		}

		lstore := makeInmemStorer(t, opts)
		if lstore == nil {
			t.Fatalf("storer should be instantiated")
		}
	})
	t.Run("disk default options", func(t *testing.T) {
		t.Parallel()

		lstore := makeDiskStorer(t, nil)
		if lstore == nil {
			t.Fatalf("storer should be instantiated")
		}
	})
	t.Run("disk with options", func(t *testing.T) {
		t.Parallel()

		opts := storer.DefaultOptions()
		opts.CacheCapacity = 10
		opts.Logger = log.Noop

		lstore := makeDiskStorer(t, opts)
		if lstore == nil {
			t.Fatalf("storer should be instantiated")
		}
	})

	t.Run("migration on latest version", func(t *testing.T) {
		t.Parallel()

		t.Run("inmem", func(t *testing.T) {
			t.Parallel()

			lstore := makeInmemStorer(t, nil)
			assertStorerVersion(t, lstore)
		})

		t.Run("disk", func(t *testing.T) {
			t.Parallel()

			lstore := makeDiskStorer(t, nil)
			assertStorerVersion(t, lstore)
		})
	})
}

func assertStorerVersion(t *testing.T, lstore *storer.DB) {
	t.Helper()

	current, err := migration.Version(lstore.Repo().IndexStore())
	if err != nil {
		t.Fatalf("migration.Version(...): unexpected error: %v", err)
	}

	expected := migration.LatestVersion(localmigration.AllSteps())

	if current != expected {
		t.Fatalf("storer is not migrated to latest version; got %d, expected %d", current, expected)
	}
}

func makeInmemStorer(t *testing.T, opts *storer.Options) *storer.DB {
	t.Helper()

	lstore, err := storer.New("", nil)
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

	dir, err := ioutil.TempDir(".", "testrepo*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		err := os.RemoveAll(dir)
		if err != nil {
			t.Errorf("failed removing directories: %v", err)
		}
	})

	lstore, err := storer.New(dir, opts)
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
