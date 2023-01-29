package localstore_test

import (
	"context"
	"errors"
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
			t.Fatal(err)
		}

		if hasFound != has {
			t.Fatalf("unexpected chunk state, exp has chunk %t got %t", has, hasFound)
		}
	}

	if has {
		tagInfo, err := upload.GetTagInfo(repo.IndexStore(), sessionID)
		if err != nil {
			t.Fatal(err)
		}

		if tagInfo.Split != uint64(len(chunks)) {
			t.Fatalf("unexpected split chunk count in tag, exp %d found %d", len(chunks), tagInfo.Split)
		}
		if tagInfo.Seen != 0 {
			t.Fatalf("unexpected seen chunk count in tag, exp %d found %d", len(chunks), tagInfo.Seen)
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
		t.Fatal(err)
	}

	if hasFound != has {
		t.Fatalf("unexpected pin collection state, exp exists %t got %t", has, hasFound)
	}

	allChunks := append([]swarm.Chunk{root}, chunks...)

	for _, ch := range allChunks {
		hasFound, err := repo.ChunkStore().Has(context.TODO(), ch.Address())
		if err != nil {
			t.Fatal(err)
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
				t.Fatal(err)
			}
			if id != uint64(i) {
				t.Fatalf("incorrect id generated, exp %d found %d", i, id)
			}
		}
	})

	t.Run("error on no tag", func(t *testing.T) {
		t.Parallel()

		lstore, err := newLocalstore()
		if err != nil {
			t.Fatal(err)
		}

		_, err = lstore.Upload(context.TODO(), false, 0)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	for _, tc := range []struct {
		name   string
		root   swarm.Chunk
		chunks []swarm.Chunk
		pin    bool
		fail   bool
	}{
		{
			name:   "10 chunks",
			root:   chunktesting.GenerateTestRandomChunk(),
			chunks: chunktesting.GenerateTestRandomChunks(9),
		},
		{
			name:   "20 chunks",
			root:   chunktesting.GenerateTestRandomChunk(),
			chunks: chunktesting.GenerateTestRandomChunks(19),
			fail:   true,
		},
		{
			name:   "30 chunks",
			root:   chunktesting.GenerateTestRandomChunk(),
			chunks: chunktesting.GenerateTestRandomChunks(29),
		},
		{
			name:   "10 chunks with pin",
			root:   chunktesting.GenerateTestRandomChunk(),
			chunks: chunktesting.GenerateTestRandomChunks(9),
			pin:    true,
		},
		{
			name:   "20 chunks with pin",
			root:   chunktesting.GenerateTestRandomChunk(),
			chunks: chunktesting.GenerateTestRandomChunks(19),
			pin:    true,
			fail:   true,
		},
		{
			name:   "30 chunks with pin",
			root:   chunktesting.GenerateTestRandomChunk(),
			chunks: chunktesting.GenerateTestRandomChunks(29),
			pin:    true,
		},
	} {
		tc := tc
		testName := "upload_" + tc.name
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
				t.Fatal(err)
			}

			session, err := lstore.Upload(context.TODO(), tc.pin, id)
			if err != nil {
				t.Fatal(err)
			}

			for _, ch := range append([]swarm.Chunk{tc.root}, tc.chunks...) {
				err := session.Put(context.TODO(), ch)
				if err != nil {
					t.Fatal(err)
				}
			}

			if tc.fail {
				err := session.Cleanup()
				if err != nil {
					t.Fatal(err)
				}
			} else {
				err := session.Done(tc.root.Address())
				if err != nil {
					t.Fatal(err)
				}
			}
			verifySessionInfo(t, lstore.Repo(), id, append([]swarm.Chunk{tc.root}, tc.chunks...), !tc.fail)
			if tc.pin {
				verifyPinCollection(t, lstore.Repo(), tc.root, tc.chunks, !tc.fail)
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
				t.Fatalf("unexpected TagID in session, exp %d found %d", id, info.TagID)
			}

			if info.Split != split {
				t.Fatalf("unexpected split count in session, exp %d found %d", split, info.Split)
			}

			if info.Seen != seen {
				t.Fatalf("unexpected seen count in session, exp %d found %d", seen, info.Seen)
			}

			if !info.Address.Equal(addr) {
				t.Fatalf("unexpected swarm reference, exp %s, found %s", addr, info.Address)
			}
		}

		t.Run("commit", func(t *testing.T) {
			id, err := lstore.NewSession()
			if err != nil {
				t.Fatal(err)
			}

			session, err := lstore.Upload(context.TODO(), false, id)
			if err != nil {
				t.Fatal(err)
			}

			sessionInfo, err := lstore.GetSessionInfo(id)
			if err != nil {
				t.Fatal(err)
			}

			verify(t, sessionInfo, id, 0, 0, swarm.ZeroAddress)

			chunks := chunktesting.GenerateTestRandomChunks(10)

			for _, ch := range chunks {
				for i := 0; i < 2; i++ {
					err := session.Put(context.TODO(), ch)
					if err != nil {
						t.Fatal(err)
					}
				}
			}

			err = session.Done(chunks[0].Address())
			if err != nil {
				t.Fatal(err)
			}

			sessionInfo, err = lstore.GetSessionInfo(id)
			if err != nil {
				t.Fatal(err)
			}

			verify(t, sessionInfo, id, 20, 10, chunks[0].Address())
		})

		t.Run("rollback", func(t *testing.T) {
			id, err := lstore.NewSession()
			if err != nil {
				t.Fatal(err)
			}

			session, err := lstore.Upload(context.TODO(), false, id)
			if err != nil {
				t.Fatal(err)
			}

			sessionInfo, err := lstore.GetSessionInfo(id)
			if err != nil {
				t.Fatal(err)
			}

			verify(t, sessionInfo, id, 0, 0, swarm.ZeroAddress)

			chunks := chunktesting.GenerateTestRandomChunks(10)

			for _, ch := range chunks {
				err := session.Put(context.TODO(), ch)
				if err != nil {
					t.Fatal(err)
				}
			}

			err = session.Cleanup()
			if err != nil {
				t.Fatal(err)
			}

			sessionInfo, err = lstore.GetSessionInfo(id)
			if !errors.Is(err, storage.ErrNotFound) {
				t.Fatalf("expected ErrNotFound, got: %v", err)
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
		t.Cleanup(func() { os.RemoveAll(dir) })

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
	testCases := []struct {
		name   string
		root   swarm.Chunk
		chunks []swarm.Chunk
		fail   bool
	}{
		{
			name:   "10 chunks",
			root:   chunktesting.GenerateTestRandomChunk(),
			chunks: chunktesting.GenerateTestRandomChunks(9),
		},
		{
			name:   "20 chunks",
			root:   chunktesting.GenerateTestRandomChunk(),
			chunks: chunktesting.GenerateTestRandomChunks(19),
			fail:   true,
		},
		{
			name:   "30 chunks",
			root:   chunktesting.GenerateTestRandomChunk(),
			chunks: chunktesting.GenerateTestRandomChunks(29),
		},
	}

	lstore, err := newLocalstore()
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range testCases {
		testName := "pin_" + tc.name
		if tc.fail {
			testName += "_rollback"
		}
		t.Run(testName, func(t *testing.T) {
			session, err := lstore.NewCollection(context.TODO())
			if err != nil {
				t.Fatal(err)
			}

			for _, ch := range append([]swarm.Chunk{tc.root}, tc.chunks...) {
				err := session.Put(context.TODO(), ch)
				if err != nil {
					t.Fatal(err)
				}
			}

			if tc.fail {
				err := session.Cleanup()
				if err != nil {
					t.Fatal(err)
				}
			} else {
				err := session.Done(tc.root.Address())
				if err != nil {
					t.Fatal(err)
				}
			}
			verifyPinCollection(t, lstore.Repo(), tc.root, tc.chunks, !tc.fail)
		})
	}

	for _, tc := range testCases {
		t.Run("has "+tc.root.Address().String(), func(t *testing.T) {
			hasFound, err := lstore.HasPin(tc.root.Address())
			if err != nil {
				t.Fatal(err)
			}
			if hasFound != !tc.fail {
				t.Fatalf("unexpected chunk state, exp has chunk %t got %t", !tc.fail, hasFound)
			}
		})
	}

	t.Run("pins", func(t *testing.T) {
		pins, err := lstore.Pins()
		if err != nil {
			t.Fatal(err)
		}

		if len(pins) != 2 {
			t.Fatalf("unexpected no of pins, exp 2 found %d", len(pins))
		}
	})

	t.Run("delete pin", func(t *testing.T) {
		err := lstore.DeletePin(context.TODO(), testCases[2].root.Address())
		if err != nil {
			t.Fatal(err)
		}

		has, err := lstore.HasPin(testCases[2].root.Address())
		if err != nil {
			t.Fatal(err)
		}
		if has {
			t.Fatal("expected root pin reference to be deleted")
		}

		pins, err := lstore.Pins()
		if err != nil {
			t.Fatal(err)
		}
		if len(pins) != 1 {
			t.Fatalf("unexpected length of pins, exp 1, found %d", len(pins))
		}

		for _, ch := range append([]swarm.Chunk{testCases[2].root}, testCases[2].chunks...) {
			has, err := lstore.Repo().ChunkStore().Has(context.TODO(), ch.Address())
			if err != nil {
				t.Fatal(err)
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
