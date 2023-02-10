// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	storer "github.com/ethersphere/bee/pkg/localstorev2"
	chunktesting "github.com/ethersphere/bee/pkg/storage/testing"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
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

		testPinStore(t, func() (*storer.DB, error) { return storer.New("", nil) })
	})
	t.Run("disk", func(t *testing.T) {
		t.Parallel()

		testPinStore(t, diskStorer(t, nil))
	})
}
