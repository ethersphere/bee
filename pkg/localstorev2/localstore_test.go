// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore_test

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

	localstore "github.com/ethersphere/bee/pkg/localstorev2"
	pinstore "github.com/ethersphere/bee/pkg/localstorev2/internal/pinning"
	"github.com/ethersphere/bee/pkg/localstorev2/internal/upload"
	chunktesting "github.com/ethersphere/bee/pkg/storage/testing"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
)

func verifySessionInfo(
	t *testing.T,
	repo storage.Repository,
	sessionID uint64,
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

	for _, ch := range chunks {
		hasFound, err := repo.ChunkStore().Has(context.TODO(), ch.Address())
		if err != nil {
			t.Fatalf("ChunkStore.Has(...): unexpected error: %v", err)
		}

		if hasFound != has {
			t.Fatalf("unexpected chunk state, exp has chunk %t got %t", has, hasFound)
		}
	}
}

func testUploadStore(t *testing.T, newLocalstore func() (*localstore.DB, error)) {
	t.Helper()

	t.Run("new session", func(t *testing.T) {
		t.Parallel()

		lstore, err := newLocalstore()
		if err != nil {
			t.Fatal(err)
		}

		for i := 1; i < 5; i++ {
			id, err := lstore.NewSession()
			if err != nil {
				t.Fatalf("NewSession(): unexpected error: %v", err)
			}
			if id != uint64(i) {
				t.Fatalf("incorrect id generated: want %d have %d", i, id)
			}
		}
	})

	t.Run("no tag", func(t *testing.T) {
		t.Parallel()

		lstore, err := newLocalstore()
		if err != nil {
			t.Fatal(err)
		}

		_, err = lstore.Upload(context.TODO(), false, 0)
		if err == nil {
			t.Fatal("expected error on Upload with no tag")
		}
	})

	for _, tc := range []struct {
		chunks []swarm.Chunk
		pin    bool
		fail   bool
	}{
		{
			chunks: chunktesting.GenerateTestRandomChunks(10),
		},
		{
			chunks: chunktesting.GenerateTestRandomChunks(20),
			fail:   true,
		},
		{
			chunks: chunktesting.GenerateTestRandomChunks(30),
		},
		{
			chunks: chunktesting.GenerateTestRandomChunks(10),
			pin:    true,
		},
		{
			chunks: chunktesting.GenerateTestRandomChunks(20),
			pin:    true,
			fail:   true,
		},
		{
			chunks: chunktesting.GenerateTestRandomChunks(30),
			pin:    true,
		},
	} {
		tc := tc
		testName := fmt.Sprintf("upload_%d_chunks", len(tc.chunks))
		if tc.pin {
			testName += "_with_pin"
		}
		if tc.fail {
			testName += "_rollback"
		}
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			lstore, err := newLocalstore()
			if err != nil {
				t.Fatal(err)
			}

			id, err := lstore.NewSession()
			if err != nil {
				t.Fatalf("NewSession(): unexpected error: %v", err)
			}

			session, err := lstore.Upload(context.TODO(), tc.pin, id)
			if err != nil {
				t.Fatalf("Upload(...): unexpected error: %v", err)
			}

			for _, ch := range tc.chunks {
				err := session.Put(context.TODO(), ch)
				if err != nil {
					t.Fatalf("session.Put(...): unexpected error: %v", err)
				}
			}

			if tc.fail {
				err := session.Cleanup()
				if err != nil {
					t.Fatalf("session.Cleanup(): unexpected error: %v", err)
				}
			} else {
				err := session.Done(tc.chunks[0].Address())
				if err != nil {
					t.Fatalf("session.Done(...): unexpected error: %v", err)
				}
			}
			verifySessionInfo(t, lstore.Repo(), id, tc.chunks, !tc.fail)
			if tc.pin {
				verifyPinCollection(t, lstore.Repo(), tc.chunks[0], tc.chunks, !tc.fail)
			}
		})
	}

	t.Run("get session info", func(t *testing.T) {
		t.Parallel()

		lstore, err := newLocalstore()
		if err != nil {
			t.Fatal(err)
		}

		verify := func(t *testing.T, info localstore.SessionInfo, id, split, seen uint64, addr swarm.Address) {
			t.Helper()

			if info.TagID != id {
				t.Fatalf("unexpected TagID in session: want %d have %d", id, info.TagID)
			}

			if info.Split != split {
				t.Fatalf("unexpected split count in session: want %d have %d", split, info.Split)
			}

			if info.Seen != seen {
				t.Fatalf("unexpected seen count in session: want %d have %d", seen, info.Seen)
			}

			if !info.Address.Equal(addr) {
				t.Fatalf("unexpected swarm reference: want %s have %s", addr, info.Address)
			}
		}

		t.Run("done", func(t *testing.T) {
			id, err := lstore.NewSession()
			if err != nil {
				t.Fatalf("NewSession(): unexpected error: %v", err)
			}

			session, err := lstore.Upload(context.TODO(), false, id)
			if err != nil {
				t.Fatalf("Upload(...): unexpected error: %v", err)
			}

			sessionInfo, err := lstore.GetSessionInfo(id)
			if err != nil {
				t.Fatalf("GetSessionInfo(...): unexpected error: %v", err)
			}

			verify(t, sessionInfo, id, 0, 0, swarm.ZeroAddress)

			chunks := chunktesting.GenerateTestRandomChunks(10)

			for _, ch := range chunks {
				for i := 0; i < 2; i++ {
					err := session.Put(context.TODO(), ch)
					if err != nil {
						t.Fatalf("session.Put(...): unexpected error: %v", err)
					}
				}
			}

			err = session.Done(chunks[0].Address())
			if err != nil {
				t.Fatalf("session.Done(...): unexpected error: %v", err)
			}

			sessionInfo, err = lstore.GetSessionInfo(id)
			if err != nil {
				t.Fatalf("GetSessionInfo(...): unexpected error: %v", err)
			}

			verify(t, sessionInfo, id, 20, 10, chunks[0].Address())
		})

		t.Run("cleanup", func(t *testing.T) {
			id, err := lstore.NewSession()
			if err != nil {
				t.Fatalf("NewSession(): unexpected error: %v", err)
			}

			session, err := lstore.Upload(context.TODO(), false, id)
			if err != nil {
				t.Fatalf("Upload(...): unexpected error: %v", err)
			}

			sessionInfo, err := lstore.GetSessionInfo(id)
			if err != nil {
				t.Fatalf("GetSessionInfo(...): unexpected error: %v", err)
			}

			verify(t, sessionInfo, id, 0, 0, swarm.ZeroAddress)

			chunks := chunktesting.GenerateTestRandomChunks(10)

			for _, ch := range chunks {
				err := session.Put(context.TODO(), ch)
				if err != nil {
					t.Fatalf("session.Put(...): unexpected error: %v", err)
				}
			}

			err = session.Cleanup()
			if err != nil {
				t.Fatalf("session.Cleanup(): unexpected error: %v", err)
			}

			_, err = lstore.GetSessionInfo(id)
			if !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("unexpected error: want %v have %v", storage.ErrNotFound, err)
			}
		})
	})
}

// TestMain exists to adjust the time.Now function to a fixed value.
func TestMain(m *testing.M) {
	localstore.ReplaceSharkyShardLimit(4)
	code := m.Run()
	localstore.ReplaceSharkyShardLimit(32)
	os.Exit(code)
}

func diskLocalstore(t *testing.T, opts *localstore.Options) func() (*localstore.DB, error) {
	t.Helper()

	return func() (*localstore.DB, error) {
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

		lstore, err := localstore.New(dir, opts)
		if err == nil {
			t.Cleanup(func() {
				err := lstore.Close()
				if err != nil {
					t.Errorf("failed closing localstore: %v", err)
				}
			})
		}

		return lstore, err
	}
}

func TestUploadStore(t *testing.T) {
	t.Parallel()

	t.Run("inmem", func(t *testing.T) {
		t.Parallel()

		testUploadStore(t, func() (*localstore.DB, error) { return localstore.New("", nil) })
	})
	t.Run("disk", func(t *testing.T) {
		t.Parallel()

		testUploadStore(t, diskLocalstore(t, nil))
	})
}

func testPinStore(t *testing.T, newLocalstore func() (*localstore.DB, error)) {
	t.Helper()

	testCases := []struct {
		chunks []swarm.Chunk
		fail   bool
	}{
		{
			chunks: chunktesting.GenerateTestRandomChunks(10),
		},
		{
			chunks: chunktesting.GenerateTestRandomChunks(20),
			fail:   true,
		},
		{
			chunks: chunktesting.GenerateTestRandomChunks(30),
		},
	}

	lstore, err := newLocalstore()
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range testCases {
		testName := fmt.Sprintf("pin_%d_chunks", len(tc.chunks))
		if tc.fail {
			testName += "_rollback"
		}
		t.Run(testName, func(t *testing.T) {
			session, err := lstore.NewCollection(context.TODO())
			if err != nil {
				t.Fatalf("NewCollection(...): unexpected error: %v", err)
			}

			for _, ch := range tc.chunks {
				err := session.Put(context.TODO(), ch)
				if err != nil {
					t.Fatalf("session.Put(...): unexpected error: %v", err)
					t.Fatal(err)
				}
			}

			if tc.fail {
				err := session.Cleanup()
				if err != nil {
					t.Fatalf("session.Cleanup(): unexpected error: %v", err)
				}
			} else {
				err := session.Done(tc.chunks[0].Address())
				if err != nil {
					t.Fatalf("session.Done(...): unexpected error: %v", err)
				}
			}
			verifyPinCollection(t, lstore.Repo(), tc.chunks[0], tc.chunks, !tc.fail)
		})
	}

	for _, tc := range testCases {
		t.Run("has "+tc.chunks[0].Address().String(), func(t *testing.T) {
			hasFound, err := lstore.HasPin(tc.chunks[0].Address())
			if err != nil {
				t.Fatalf("HasPin(...): unexpected error: %v", err)
			}
			if hasFound != !tc.fail {
				t.Fatalf("unexpected has chunk state: want %t have %t", !tc.fail, hasFound)
			}
		})
	}

	t.Run("pins", func(t *testing.T) {
		pins, err := lstore.Pins()
		if err != nil {
			t.Fatalf("Pins(): unexpected error: %v", err)
		}

		want := 2
		if len(pins) != want {
			t.Fatalf("unexpected no of pins: want %d have %d", want, len(pins))
		}
	})

	t.Run("delete pin", func(t *testing.T) {
		t.Run("commit", func(t *testing.T) {
			err := lstore.DeletePin(context.TODO(), testCases[2].chunks[0].Address())
			if err != nil {
				t.Fatalf("DeletePin(...): unexpected error: %v", err)
			}

			verifyPinCollection(t, lstore.Repo(), testCases[2].chunks[0], testCases[2].chunks, false)
		})
		t.Run("rollback", func(t *testing.T) {
			want := errors.New("dummy error")
			lstore.SetRepoStoreDeleteHook(func(item storage.Item) error {
				// return error for delete of second last item in collection
				// this should trigger a rollback
				if item.ID() == testCases[0].chunks[8].Address().ByteString() {
					return want
				}
				return nil
			})

			have := lstore.DeletePin(context.TODO(), testCases[0].chunks[0].Address())
			if !errors.Is(have, want) {
				t.Fatalf("DeletePin(...): unexpected error: want %v have %v", want, have)
			}

			verifyPinCollection(t, lstore.Repo(), testCases[0].chunks[0], testCases[0].chunks, true)
		})
	})
}

func TestPinStore(t *testing.T) {
	t.Parallel()

	t.Run("inmem", func(t *testing.T) {
		t.Parallel()

		testPinStore(t, func() (*localstore.DB, error) { return localstore.New("", nil) })
	})
	t.Run("disk", func(t *testing.T) {
		t.Parallel()

		testPinStore(t, diskLocalstore(t, nil))
	})
}

func testCacheStore(t *testing.T, newLocalstore func() (*localstore.DB, error)) {
	t.Helper()

	chunks := chunktesting.GenerateTestRandomChunks(9)

	lstore, err := newLocalstore()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("cache chunks", func(t *testing.T) {
		t.Run("commit", func(t *testing.T) {
			putter := lstore.Cache()
			for _, ch := range chunks {
				err := putter.Put(context.TODO(), ch)
				if err != nil {
					t.Fatalf("Cache.Put(...): unexpected error: %v", err)
				}
			}
		})

		t.Run("rollback", func(t *testing.T) {
			want := errors.New("dummy error")
			lstore.SetRepoStorePutHook(func(item storage.Item) error {
				if item.Namespace() == "cacheState" {
					return want
				}
				return nil
			})
			errChunk := chunktesting.GenerateTestRandomChunk()
			have := lstore.Cache().Put(context.TODO(), errChunk)
			if !errors.Is(have, want) {
				t.Fatalf("unexpected error on cache put: want %v have %v", want, have)
			}
			haveChunk, err := lstore.Repo().ChunkStore().Has(context.TODO(), errChunk.Address())
			if err != nil {
				t.Fatalf("ChunkStore.Has(...): unexpected error: %v", err)
			}
			if haveChunk {
				t.Fatalf("unexpected chunk state: want false have %t", haveChunk)
			}
		})
	})
	t.Run("lookup", func(t *testing.T) {
		t.Run("commit", func(t *testing.T) {
			lstore.SetRepoStorePutHook(nil)
			getter := lstore.Lookup()
			for _, ch := range chunks {
				have, err := getter.Get(context.TODO(), ch.Address())
				if err != nil {
					t.Fatalf("Cache.Get(...): unexpected error: %v", err)
				}
				if !have.Equal(ch) {
					t.Fatalf("chunk %s does not match", ch.Address())
				}
			}
		})
		t.Run("rollback", func(t *testing.T) {
			want := errors.New("dummy error")
			lstore.SetRepoStorePutHook(func(item storage.Item) error {
				if item.Namespace() == "cacheState" {
					return want
				}
				return nil
			})
			// fail access for the first 4 chunks. This will keep the order as is
			// from the last test.
			for idx, ch := range chunks {
				if idx > 4 {
					break
				}
				_, have := lstore.Lookup().Get(context.TODO(), ch.Address())
				if !errors.Is(have, want) {
					t.Fatalf("unexpected error in cache get: want %v have %v", want, have)
				}
			}
		})
	})
	t.Run("cache chunks beyond capacity", func(t *testing.T) {
		lstore.SetRepoStorePutHook(nil)
		// add chunks beyond capacity and verify the correct chunks are removed
		// from the cache based on last access order
		newChunks := chunktesting.GenerateTestRandomChunks(5)
		putter := lstore.Cache()
		for _, ch := range newChunks {
			err := putter.Put(context.TODO(), ch)
			if err != nil {
				t.Fatalf("Cache.Put(...): unexpected error: %v", err)
			}
		}

		for idx, ch := range append(chunks, newChunks...) {
			var want error = nil
			readCh, have := lstore.Lookup().Get(context.TODO(), ch.Address())
			if idx < 4 {
				want = storage.ErrNotFound
			}
			if !errors.Is(have, want) {
				t.Fatalf("unexpected error on Get: idx %d want %v have %v", idx, want, have)
			}
			if have == nil {
				if !readCh.Equal(ch) {
					t.Fatalf("incorrect chunk data read for %s", readCh.Address())
				}
			}
		}
	})
}

func TestCacheStore(t *testing.T) {
	t.Parallel()

	t.Run("inmem", func(t *testing.T) {
		t.Parallel()

		testCacheStore(t, func() (*localstore.DB, error) {
			return localstore.New("", &localstore.Options{CacheCapacity: 10})
		})
	})
	t.Run("disk", func(t *testing.T) {
		t.Parallel()

		opts := localstore.DefaultOptions()
		opts.CacheCapacity = 10

		testCacheStore(t, diskLocalstore(t, opts))
	})
}

func TestPushSubscriber(t *testing.T) {
	t.Parallel()

	t.Run("inmem", func(t *testing.T) {
		t.Parallel()

		testPushSubscriber(t, func() (*localstore.DB, error) {
			return localstore.New("", &localstore.Options{CacheCapacity: 10})
		})
	})
	t.Run("disk", func(t *testing.T) {
		t.Skip("wip")
		t.Parallel()

		opts := localstore.DefaultOptions()
		opts.CacheCapacity = 10

		testPushSubscriber(t, diskLocalstore(t, opts))
	})
}

func testPushSubscriber(t *testing.T, newLocalstore func() (*localstore.DB, error)) {
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

		defer p.Cleanup()

		ch := chunktesting.GenerateTestRandomChunks(count)
		for i := 0; i < count; i++ {
			if err := p.Put(context.TODO(), ch[i]); err != nil {
				t.Fatal(err)
			}

			defer p.Done(ch[i].Address())

			chunks = append(chunks, ch[i])

			chunkProcessedTimes = append(chunkProcessedTimes, 0)
		}
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
			i                   int // address index
			gotStamp, wantStamp []byte
		)
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

	for i := 0; i < wantedChunksCount; i++ {
		select {
		case err := <-errChan:
			if err != nil {
				t.Error(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}
