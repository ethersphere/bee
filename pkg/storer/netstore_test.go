// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ethersphere/bee/v2/pkg/pusher"
	"github.com/ethersphere/bee/v2/pkg/pushsync"
	"github.com/ethersphere/bee/v2/pkg/retrieval"
	storage "github.com/ethersphere/bee/v2/pkg/storage"
	chunktesting "github.com/ethersphere/bee/v2/pkg/storage/testing"
	storer "github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type testRetrieval struct {
	fn func(swarm.Address) (swarm.Chunk, error)
}

func (t *testRetrieval) RetrieveChunk(_ context.Context, address swarm.Address, _ swarm.Address) (swarm.Chunk, error) {
	return t.fn(address)
}

func testNetStore(t *testing.T, newStorer func(r retrieval.Interface) (*storer.DB, error)) {
	t.Helper()

	t.Run("direct upload", func(t *testing.T) {
		t.Parallel()

		t.Run("commit", func(t *testing.T) {
			t.Parallel()

			chunks := chunktesting.GenerateTestRandomChunks(10)

			lstore, err := newStorer(nil)
			if err != nil {
				t.Fatal(err)
			}

			session := lstore.DirectUpload()

			count := 0
			quit := make(chan struct{})
			t.Cleanup(func() { close(quit) })
			go func() {
				for {
					select {
					case op := <-lstore.PusherFeed():
						found := slices.ContainsFunc(chunks, op.Chunk.Equal)
						if !found {
							op.Err <- fmt.Errorf("incorrect chunk for push: have %s", op.Chunk.Address())
							continue
						}
						count++
						op.Err <- nil
					case <-quit:
						return
					}
				}
			}()

			for _, ch := range chunks {
				err := session.Put(context.TODO(), ch)
				if err != nil {
					t.Fatalf("session.Put(...): unexpected error: %v", err)
				}
			}

			err = session.Done(chunks[0].Address())
			if err != nil {
				t.Fatalf("session.Done(): unexpected error: %v", err)
			}

			if count != 10 {
				t.Fatalf("unexpected no of pusher ops want 10 have %d", count)
			}

			verifyChunks(t, lstore.Storage(), chunks, false)
		})

		t.Run("pusher error", func(t *testing.T) {
			t.Parallel()

			chunks := chunktesting.GenerateTestRandomChunks(10)

			lstore, err := newStorer(nil)
			if err != nil {
				t.Fatal(err)
			}

			session := lstore.DirectUpload()

			count := 0
			quit := make(chan struct{})
			t.Cleanup(func() { close(quit) })
			wantErr := errors.New("dummy error")
			go func() {
				for {
					select {
					case op := <-lstore.PusherFeed():
						found := slices.ContainsFunc(chunks, op.Chunk.Equal)
						if !found {
							op.Err <- fmt.Errorf("incorrect chunk for push: have %s", op.Chunk.Address())
							continue
						}
						count++
						if count >= 5 {
							op.Err <- wantErr
						} else {
							op.Err <- nil
						}
					case <-quit:
						return
					}
				}
			}()

			for _, ch := range chunks {
				err := session.Put(context.TODO(), ch)
				if err != nil && !errors.Is(err, wantErr) {
					t.Fatalf("session.Put(...): unexpected error: %v", err)
				}
			}

			err = session.Cleanup()
			if err != nil {
				t.Fatalf("session.Cleanup(): unexpected error: %v", err)
			}

			verifyChunks(t, lstore.Storage(), chunks, false)
		})

		t.Run("context cancellation", func(t *testing.T) {
			t.Parallel()

			chunks := chunktesting.GenerateTestRandomChunks(10)

			lstore, err := newStorer(nil)
			if err != nil {
				t.Fatal(err)
			}

			session := lstore.DirectUpload()

			ctx, cancel := context.WithCancel(context.Background())

			count := 0
			go func() {
				<-lstore.PusherFeed()
				count++
				cancel()
			}()

			for _, ch := range chunks {
				err := session.Put(ctx, ch)
				if err != nil && !errors.Is(err, context.Canceled) {
					t.Fatalf("session.Put(...): unexpected error: have %v", err)
				}
			}

			err = session.Cleanup()
			if err != nil {
				t.Fatalf("session.Cleanup(): unexpected error: %v", err)
			}

			if count != 1 {
				t.Fatalf("unexpected no of pusher ops want 5 have %d", count)
			}

			verifyChunks(t, lstore.Storage(), chunks, false)
		})

		t.Run("shallow receipt retry", func(t *testing.T) {
			t.Parallel()

			chunk := chunktesting.GenerateTestRandomChunk()

			lstore, err := newStorer(nil)
			if err != nil {
				t.Fatal(err)
			}

			count := 3
			go func() {
				for op := range lstore.PusherFeed() {
					if !op.Chunk.Equal(chunk) {
						op.Err <- fmt.Errorf("incorrect chunk for push: have %s", op.Chunk.Address())
						continue
					}
					if count > 0 {
						count--
						op.Err <- pushsync.ErrShallowReceipt
					} else {
						op.Err <- nil
					}
				}
			}()

			session := lstore.DirectUpload()

			err = session.Put(context.Background(), chunk)
			if err != nil {
				t.Fatalf("session.Put(...): unexpected error: %v", err)
			}

			err = session.Done(chunk.Address())
			if err != nil {
				t.Fatalf("session.Done(): unexpected error: %v", err)
			}

			if count != 0 {
				t.Fatalf("unexpected no of pusher ops want 0 have %d", count)
			}
		})

		t.Run("could not sync", func(t *testing.T) {
			t.Parallel()

			chunk := chunktesting.GenerateTestRandomChunk()

			lstore, err := newStorer(nil)
			if err != nil {
				t.Fatal(err)
			}

			// the pusher gives up after a few shallow receipts; the upload
			// must end with an error instead of retrying forever
			shallow := 3
			go func() {
				for op := range lstore.PusherFeed() {
					if shallow > 0 {
						shallow--
						op.Err <- pushsync.ErrShallowReceipt
					} else {
						op.Err <- pusher.ErrCouldNotSync
					}
				}
			}()

			session := lstore.DirectUpload()

			err = session.Put(context.Background(), chunk)
			if err != nil {
				t.Fatalf("session.Put(...): unexpected error: %v", err)
			}

			err = session.Done(chunk.Address())
			if !errors.Is(err, pusher.ErrCouldNotSync) {
				t.Fatalf("session.Done(): got %v, want %v", err, pusher.ErrCouldNotSync)
			}
		})

		t.Run("download", func(t *testing.T) {
			t.Parallel()

			t.Run("with cache", func(t *testing.T) {
				t.Parallel()

				chunks := chunktesting.GenerateTestRandomChunks(10)

				lstore, err := newStorer(&testRetrieval{fn: func(address swarm.Address) (swarm.Chunk, error) {
					for _, ch := range chunks[5:] {
						if ch.Address().Equal(address) {
							return ch, nil
						}
					}
					return nil, storage.ErrNotFound
				}})
				if err != nil {
					t.Fatal(err)
				}

				// Add some chunks to Cache to simulate local retrieval.
				for idx, ch := range chunks {
					if idx < 5 {
						err := lstore.Cache().Put(context.TODO(), ch)
						if err != nil {
							t.Fatalf("cache.Put(...): unexpected error: %v", err)
						}
					} else {
						break
					}
				}

				getter := lstore.Download(true)

				for idx, ch := range chunks {
					readCh, err := getter.Get(context.TODO(), ch.Address())
					if err != nil {
						t.Fatalf("download.Get(...): unexpected error: %v idx %d", err, idx)
					}
					if !readCh.Equal(ch) {
						t.Fatalf("incorrect chunk read: address %s", readCh.Address())
					}
				}

				t.Cleanup(lstore.WaitForBgCacheWorkers())

				// After download is complete all chunks should be in the local storage.
				verifyChunks(t, lstore.Storage(), chunks, true)
			})
		})

		t.Run("no cache", func(t *testing.T) {
			t.Parallel()

			chunks := chunktesting.GenerateTestRandomChunks(10)

			lstore, err := newStorer(&testRetrieval{fn: func(address swarm.Address) (swarm.Chunk, error) {
				for _, ch := range chunks[5:] {
					if ch.Address().Equal(address) {
						return ch, nil
					}
				}
				return nil, storage.ErrNotFound
			}})
			if err != nil {
				t.Fatal(err)
			}

			// Add some chunks to Cache to simulate local retrieval.
			for idx, ch := range chunks {
				if idx < 5 {
					err := lstore.Cache().Put(context.TODO(), ch)
					if err != nil {
						t.Fatalf("cache.Put(...): unexpected error: %v", err)
					}
				} else {
					break
				}
			}

			getter := lstore.Download(false)

			for _, ch := range chunks {
				readCh, err := getter.Get(context.TODO(), ch.Address())
				if err != nil {
					t.Fatalf("download.Get(...): unexpected error: %v", err)
				}
				if !readCh.Equal(ch) {
					t.Fatalf("incorrect chunk read: address %s", readCh.Address())
				}
			}

			// only the chunks that were already in cache should be present
			verifyChunks(t, lstore.Storage(), chunks[:5], true)
			verifyChunks(t, lstore.Storage(), chunks[5:], false)
		})
	})
}

func TestNetStore(t *testing.T) {
	t.Parallel()

	t.Run("inmem", func(t *testing.T) {
		t.Parallel()

		testNetStore(t, func(r retrieval.Interface) (*storer.DB, error) {
			opts := dbTestOps(swarm.RandAddress(t), 0, nil, nil, time.Second)
			opts.CacheCapacity = 100

			db, err := storer.New(context.Background(), "", opts)
			if err == nil {
				db.SetRetrievalService(r)
			}
			return db, err
		})
	})
	t.Run("disk", func(t *testing.T) {
		t.Parallel()

		testNetStore(t, func(r retrieval.Interface) (*storer.DB, error) {
			opts := dbTestOps(swarm.RandAddress(t), 0, nil, nil, time.Second)

			db, err := diskStorer(t, opts)()
			if err == nil {
				db.SetRetrievalService(r)
			}
			return db, err
		})
	})
}

// TestDirectUploadStopsOnError verifies that once a chunk fails a direct
// upload, the upload stops: later chunks are not pushed and the next put
// returns the error.
func TestDirectUploadStopsOnError(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		chunks := chunktesting.GenerateTestRandomChunks(2)

		lstore, err := storer.New(context.Background(), "", dbTestOps(swarm.RandAddress(t), 0, nil, nil, time.Second))
		if err != nil {
			t.Fatal(err)
		}

		var pushed []swarm.Address
		quit := make(chan struct{})
		defer func() {
			close(quit)
			if err := lstore.Close(); err != nil {
				t.Errorf("close: %v", err)
			}
			// leveldb waits up to a second for its memory pool on close
			time.Sleep(time.Second)
		}()
		go func() {
			for {
				select {
				case op := <-lstore.PusherFeed():
					pushed = append(pushed, op.Chunk.Address())
					op.Err <- pushsync.ErrNoPush
				case <-quit:
					return
				}
			}
		}()

		session := lstore.DirectUpload()

		if err := session.Put(context.Background(), chunks[0]); err != nil {
			t.Fatalf("session.Put(...): unexpected error: %v", err)
		}
		synctest.Wait()

		if err := session.Put(context.Background(), chunks[1]); !errors.Is(err, pushsync.ErrNoPush) {
			t.Fatalf("session.Put(...): got %v, want %v", err, pushsync.ErrNoPush)
		}
		synctest.Wait()

		if len(pushed) != 1 || !pushed[0].Equal(chunks[0].Address()) {
			t.Fatalf("pushed %v, want only %s", pushed, chunks[0].Address())
		}

		if err := session.Done(chunks[1].Address()); !errors.Is(err, pushsync.ErrNoPush) {
			t.Fatalf("session.Done(): got %v, want %v", err, pushsync.ErrNoPush)
		}
	})
}
