// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	chunktesting "github.com/ethersphere/bee/pkg/storage/testing"
	storer "github.com/ethersphere/bee/pkg/storer"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/google/go-cmp/cmp"
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
			tag, err := lstore.NewSession()
			if err != nil {
				t.Fatalf("NewSession(): unexpected error: %v", err)
			}
			if tag.TagID != uint64(i) {
				t.Fatalf("incorrect id generated: want %d have %d", i, tag.TagID)
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

			tag, err := lstore.NewSession()
			if err != nil {
				t.Fatalf("NewSession(): unexpected error: %v", err)
			}

			session, err := lstore.Upload(context.TODO(), tc.pin, tag.TagID)
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
			verifySessionInfo(t, lstore.Repo(), tag.TagID, tc.chunks, !tc.fail)
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
			tag, err := lstore.NewSession()
			if err != nil {
				t.Fatalf("NewSession(): unexpected error: %v", err)
			}

			session, err := lstore.Upload(context.TODO(), false, tag.TagID)
			if err != nil {
				t.Fatalf("Upload(...): unexpected error: %v", err)
			}

			sessionInfo, err := lstore.Session(tag.TagID)
			if err != nil {
				t.Fatalf("Session(...): unexpected error: %v", err)
			}

			verify(t, sessionInfo, tag.TagID, 0, 0, swarm.ZeroAddress)

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

			sessionInfo, err = lstore.Session(tag.TagID)
			if err != nil {
				t.Fatalf("Session(...): unexpected error: %v", err)
			}

			verify(t, sessionInfo, tag.TagID, 20, 10, chunks[0].Address())
		})

		t.Run("cleanup", func(t *testing.T) {
			tag, err := lstore.NewSession()
			if err != nil {
				t.Fatalf("NewSession(): unexpected error: %v", err)
			}

			session, err := lstore.Upload(context.TODO(), false, tag.TagID)
			if err != nil {
				t.Fatalf("Upload(...): unexpected error: %v", err)
			}

			sessionInfo, err := lstore.Session(tag.TagID)
			if err != nil {
				t.Fatalf("Session(...): unexpected error: %v", err)
			}

			verify(t, sessionInfo, tag.TagID, 0, 0, swarm.ZeroAddress)

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

			got, err := lstore.Session(tag.TagID)
			if err != nil {
				t.Fatalf("Session(...): unexpected error: %v", err)
			}

			// All updates to tag should be reverted
			if diff := cmp.Diff(tag, got); diff != "" {
				t.Fatalf("tag mismatch (-want +have):\n%s", diff)
			}
		})
	})
}

func TestUploadStore(t *testing.T) {
	t.Parallel()

	t.Run("inmem", func(t *testing.T) {
		t.Parallel()

		testUploadStore(t, func() (*storer.DB, error) {
			return storer.New(context.Background(), "", dbTestOps(swarm.RandAddress(t), 0, nil, nil, time.Second))
		})
	})
	t.Run("disk", func(t *testing.T) {
		t.Parallel()

		testUploadStore(t, diskStorer(t, dbTestOps(swarm.RandAddress(t), 0, nil, nil, time.Second)))
	})
}
