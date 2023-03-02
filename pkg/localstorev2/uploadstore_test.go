// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	storer "github.com/ethersphere/bee/pkg/localstorev2"
	chunktesting "github.com/ethersphere/bee/pkg/storage/testing"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/swarm/test"
)

func testUploadStore(t *testing.T, newStorer func() (*storer.DB, error)) {
	t.Helper()

	t.Run("new session", func(t *testing.T) {
		t.Parallel()

		lstore, err := newStorer()
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

		lstore, err := newStorer()
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

			lstore, err := newStorer()
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

		lstore, err := newStorer()
		if err != nil {
			t.Fatal(err)
		}

		verify := func(t *testing.T, info storer.SessionInfo, id, split, seen uint64, addr swarm.Address) {
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

func TestUploadStore(t *testing.T) {
	t.Parallel()

	t.Run("inmem", func(t *testing.T) {
		t.Parallel()

		testUploadStore(t, func() (*storer.DB, error) {
			return storer.New(context.Background(), "", dbTestOps(test.RandomAddress(), 0, nil, nil, nil, time.Second))
		})
	})
	t.Run("disk", func(t *testing.T) {
		t.Parallel()

		testUploadStore(t, diskStorer(t, dbTestOps(test.RandomAddress(), 0, nil, nil, nil, time.Second)))
	})
}
