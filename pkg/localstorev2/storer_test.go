// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"testing"
	"time"

	storer "github.com/ethersphere/bee/pkg/localstorev2"
	pinstore "github.com/ethersphere/bee/pkg/localstorev2/internal/pinning"
	"github.com/ethersphere/bee/pkg/localstorev2/internal/upload"
	localmigration "github.com/ethersphere/bee/pkg/localstorev2/migration"
	"github.com/ethersphere/bee/pkg/log"
	chunktesting "github.com/ethersphere/bee/pkg/storage/testing"
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

		lstore, err := storer.New(context.Background(), dir, opts)
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

	lstore, err := storer.New(context.Background(), "", nil)
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

	lstore, err := storer.New(context.Background(), dir, opts)
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
func TestPushSubscriber(t *testing.T) {
	t.Parallel()

	t.Run("inmem", func(t *testing.T) {
		t.Parallel()

		testPushSubscriber(t, func() (*storer.DB, error) {
			return storer.New(context.TODO(), "", &storer.Options{
				Logger: log.Noop,
			})
		})
	})
	t.Run("disk", func(t *testing.T) {
		t.Parallel()

		opts := storer.DefaultOptions()
		opts.CacheCapacity = 10
		opts.Logger = log.Noop

		testPushSubscriber(t, diskStorer(t, opts))
	})
}

func testPushSubscriber(t *testing.T, newLocalstore func() (*storer.DB, error)) {
	t.Helper()

	lstore, err := newLocalstore()
	if err != nil {
		t.Fatal(err)
	}

	chunks := make([]swarm.Chunk, 0)
	var chunksMu sync.Mutex

	chunkProcessedTimes := make([]int, 0)

	uploadRandomChunks := func(count int) {
		chunksMu.Lock()
		defer chunksMu.Unlock()

		id, err := lstore.NewSession()
		if err != nil {
			t.Fatal(err)
		}

		p, err := lstore.Upload(context.TODO(), false, id)
		if err != nil {
			t.Fatal(err)
		}

		ch := chunktesting.GenerateTestRandomChunks(count)
		var i = 0
		for ; i < count; i++ {
			if err := p.Put(context.TODO(), ch[i]); err != nil {
				t.Fatal(err)
			}
			chunks = append(chunks, ch[i])
			chunkProcessedTimes = append(chunkProcessedTimes, 0)
		}

		_ = p.Done(ch[i-1].Address())
	}

	// prepopulate database with some chunks
	// before the subscription
	uploadRandomChunks(10)

	// set a timeout on subscription
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// collect all errors from validating addresses, even nil ones
	// to validate the number of addresses received by the subscription
	errChan := make(chan error)

	ch, _, stop := lstore.SubscribePush(ctx)
	defer stop()

	// receive and validate addresses from the subscription
	go func() {
		var (
			err, ierr           error
			gotStamp, wantStamp []byte
		)
		var i int // address index
		for {
			select {
			case got, ok := <-ch:
				if !ok {
					return
				}
				chunksMu.Lock()
				cIndex := i
				want := chunks[cIndex]
				chunkProcessedTimes[cIndex]++
				chunksMu.Unlock()
				if !bytes.Equal(got.Data(), want.Data()) {
					err = fmt.Errorf("got chunk %v data %x, want %x", i, got.Data(), want.Data())
				}
				if !got.Address().Equal(want.Address()) {
					err = fmt.Errorf("got chunk %v address %s, want %s", i, got.Address(), want.Address())
				}
				if gotStamp, ierr = got.Stamp().MarshalBinary(); ierr != nil {
					err = ierr
				}
				if wantStamp, ierr = want.Stamp().MarshalBinary(); ierr != nil {
					err = ierr
				}
				if !bytes.Equal(gotStamp, wantStamp) {
					err = errors.New("stamps don't match")
				}

				i++
				// send one and only one error per received address
				select {
				case errChan <- err:
				case <-ctx.Done():
					return
				}

				chunksMu.Lock()
				if i == len(chunks) {
					close(errChan)
				}
				chunksMu.Unlock()
			case <-ctx.Done():
				return
			}
		}
	}()

	// upload some chunks just after subscribe
	uploadRandomChunks(5)

	time.Sleep(200 * time.Millisecond)

	// upload some chunks after some short time
	// to ensure that subscription will include them
	// in a dynamic environment
	uploadRandomChunks(3)

	checkErrChan(ctx, t, errChan, len(chunks))

	chunksMu.Lock()
	for i, pc := range chunkProcessedTimes {
		if pc != 1 {
			t.Fatalf("chunk on address %s processed %d times, should be only once", chunks[i].Address(), pc)
		}
	}
	chunksMu.Unlock()
}

// checkErrChan expects the number of wantedChunksCount errors from errChan
// and calls t.Error for the ones that are not nil.
func checkErrChan(ctx context.Context, t *testing.T, errChan chan error, wantedChunksCount int) {
	t.Helper()

	for err := range errChan {
		if err != nil {
			t.Error(err)
		}
	}
}
