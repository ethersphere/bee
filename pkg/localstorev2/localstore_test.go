// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore_test

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	localstore "github.com/ethersphere/bee/pkg/localstorev2"
	pinstore "github.com/ethersphere/bee/pkg/localstorev2/internal/pinning"
	"github.com/ethersphere/bee/pkg/localstorev2/internal/upload"
	chunktesting "github.com/ethersphere/bee/pkg/storage/testing"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
)

func verifySessionInfo(
	t *testing.T,
	repo *storage.Repository,
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
	repo *storage.Repository,
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

func diskLocalstore(t *testing.T) func() (*localstore.DB, error) {
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

		lstore, err := localstore.New(dir, nil)
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

		testUploadStore(t, diskLocalstore(t))
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
		err := lstore.DeletePin(context.TODO(), testCases[2].chunks[0].Address())
		if err != nil {
			t.Fatalf("DeletePin(...): unexpected error: %v", err)
		}

		has, err := lstore.HasPin(testCases[2].chunks[0].Address())
		if err != nil {
			t.Fatalf("HasPin(...): unexpected error: %v", err)
		}
		if has {
			t.Fatal("expected root pin reference to be deleted")
		}

		pins, err := lstore.Pins()
		if err != nil {
			t.Fatalf("Pins(): unexpected error: %v", err)
		}
		want := 1
		if len(pins) != want {
			t.Fatalf("unexpected length of pins: want %d have %d", want, len(pins))
		}

		for _, ch := range testCases[2].chunks {
			has, err := lstore.Repo().ChunkStore().Has(context.TODO(), ch.Address())
			if err != nil {
				t.Fatalf("ChunkStore.Has(...): unexpected error: %v", err)
			}
			if has {
				t.Fatalf("expected chunk in collection to be deleted %s", ch.Address())
			}
		}
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

		testPinStore(t, diskLocalstore(t))
	})
}
