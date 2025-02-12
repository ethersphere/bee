// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	chunktesting "github.com/ethersphere/bee/v2/pkg/storage/testing"
	storer "github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

func testPinStore(t *testing.T, newStorer func() (*storer.DB, error)) {
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

	lstore, err := newStorer()
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
			verifyPinCollection(t, lstore.Storage(), tc.chunks[0], tc.chunks, !tc.fail)
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

			verifyPinCollection(t, lstore.Storage(), testCases[2].chunks[0], testCases[2].chunks, false)
		})
	})

	t.Run("duplicate parallel upload does not leave orphaned chunks", func(t *testing.T) {
		chunks := chunktesting.GenerateTestRandomChunks(4)

		session1, err := lstore.NewCollection(context.TODO())
		if err != nil {
			t.Fatalf("NewCollection(...): unexpected error: %v", err)
		}

		session2, err := lstore.NewCollection(context.TODO())
		if err != nil {
			t.Fatalf("NewCollection2(...): unexpected error: %v", err)
		}

		for _, ch := range chunks {
			err := session2.Put(context.TODO(), ch)
			if err != nil {
				t.Fatalf("session2.Put(...): unexpected error: %v", err)
				t.Fatal(err)
			}

			err = session1.Put(context.TODO(), ch)
			if err != nil {
				t.Fatalf("session1.Put(...): unexpected error: %v", err)
				t.Fatal(err)
			}
		}

		err = session1.Done(chunks[0].Address())
		if err != nil {
			t.Fatalf("session1.Done(...): unexpected error: %v", err)
		}

		err = session2.Done(chunks[0].Address())
		if err == nil {
			t.Fatalf("session2.Done(...): expected error, got nil")
		}

		if err := session2.Cleanup(); err != nil {
			t.Fatalf("session2.Done(...): unexpected error: %v", err)
		}

		verifyPinCollection(t, lstore.Storage(), chunks[0], chunks, true)
		verifyChunkRefCount(t, lstore.Storage(), chunks)
	})
}

func TestPinStore(t *testing.T) {
	t.Parallel()

	t.Run("inmem", func(t *testing.T) {
		t.Parallel()

		testPinStore(t, func() (*storer.DB, error) {
			return storer.New(context.Background(), "", dbTestOps(swarm.RandAddress(t), 0, nil, nil, time.Second))
		})
	})
	t.Run("disk", func(t *testing.T) {
		t.Parallel()

		testPinStore(t, diskStorer(t, dbTestOps(swarm.RandAddress(t), 0, nil, nil, time.Second)))
	})
}
