// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package upload_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"testing"
	"time"

	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/storagetest"
	chunktest "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/upload"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// now is a function that returns the current time and replaces time.Now.
var now = func() time.Time { return time.Unix(1234567890, 0) }

// TestMain exists to adjust the time.Now function to a fixed value.
func TestMain(m *testing.M) {
	upload.ReplaceTimeNow(now)
	code := m.Run()
	upload.ReplaceTimeNow(time.Now)
	os.Exit(code)
}

func TestPushItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		test *storagetest.ItemMarshalAndUnmarshalTest
	}{{
		name: "zero values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:       &upload.PushItem{},
			Factory:    func() storage.Item { return new(upload.PushItem) },
			MarshalErr: upload.ErrPushItemMarshalAddressIsZero,
		},
	}, {
		name: "zero address",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &upload.PushItem{
				Timestamp: 1,
				Address:   swarm.ZeroAddress,
				TagID:     1,
			},
			Factory:    func() storage.Item { return new(upload.PushItem) },
			MarshalErr: upload.ErrPushItemMarshalAddressIsZero,
		},
	}, {
		name: "nil stamp",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &upload.PushItem{
				Timestamp: 1,
				Address:   swarm.NewAddress(storagetest.MinAddressBytes[:]),
				TagID:     1,
			},
			Factory:    func() storage.Item { return new(upload.PushItem) },
			MarshalErr: upload.ErrPushItemMarshalBatchInvalid,
		},
	}, {
		name: "min values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &upload.PushItem{
				Timestamp: 0,
				Address:   swarm.NewAddress(storagetest.MinAddressBytes[:]),
				BatchID:   storagetest.MinAddressBytes[:],
				TagID:     0,
			},
			Factory: func() storage.Item { return new(upload.PushItem) },
		},
	}, {
		name: "max values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &upload.PushItem{
				Timestamp: math.MaxInt64,
				Address:   swarm.NewAddress(storagetest.MaxAddressBytes[:]),
				BatchID:   storagetest.MaxAddressBytes[:],
				TagID:     math.MaxUint64,
			},
			Factory: func() storage.Item { return new(upload.PushItem) },
		},
	}, {
		name: "random values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &upload.PushItem{
				Timestamp: rand.Int63(),
				Address:   swarm.RandAddress(t),
				BatchID:   swarm.RandAddress(t).Bytes(),
				TagID:     rand.Uint64(),
			},
			Factory: func() storage.Item { return new(upload.PushItem) },
		},
	}, {
		name: "invalid size",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &storagetest.ItemStub{
				MarshalBuf:   []byte{0xFF},
				UnmarshalBuf: []byte{0xFF},
			},
			Factory:      func() storage.Item { return new(upload.PushItem) },
			UnmarshalErr: upload.ErrPushItemUnmarshalInvalidSize,
		},
	}}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s marshal/unmarshal", tc.name), func(t *testing.T) {
			t.Parallel()

			storagetest.TestItemMarshalAndUnmarshal(t, tc.test)
		})

		t.Run(fmt.Sprintf("%s clone", tc.name), func(t *testing.T) {
			t.Parallel()

			storagetest.TestItemClone(t, &storagetest.ItemCloneTest{
				Item:    tc.test.Item,
				CmpOpts: tc.test.CmpOpts,
			})
		})
	}
}

func TestTagItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		test *storagetest.ItemMarshalAndUnmarshalTest
	}{{
		name: "zero values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:    &upload.TagItem{},
			Factory: func() storage.Item { return new(upload.TagItem) },
		},
	}, {
		name: "max values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &upload.TagItem{
				TagID:     math.MaxUint64,
				Split:     math.MaxUint64,
				Seen:      math.MaxUint64,
				Stored:    math.MaxUint64,
				Sent:      math.MaxUint64,
				Synced:    math.MaxUint64,
				Address:   swarm.NewAddress(storagetest.MaxAddressBytes[:]),
				StartedAt: math.MaxInt64,
			},
			Factory: func() storage.Item { return new(upload.TagItem) },
		},
	}, {
		name: "random values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &upload.TagItem{
				TagID:     rand.Uint64(),
				Split:     rand.Uint64(),
				Seen:      rand.Uint64(),
				Stored:    rand.Uint64(),
				Sent:      rand.Uint64(),
				Synced:    rand.Uint64(),
				Address:   swarm.RandAddress(t),
				StartedAt: rand.Int63(),
			},
			Factory: func() storage.Item { return new(upload.TagItem) },
		},
	}, {
		name: "invalid size",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &storagetest.ItemStub{
				MarshalBuf:   []byte{0xFF},
				UnmarshalBuf: []byte{0xFF},
			},
			Factory:      func() storage.Item { return new(upload.TagItem) },
			UnmarshalErr: upload.ErrTagItemUnmarshalInvalidSize,
		},
	}}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s marshal/unmarshal", tc.name), func(t *testing.T) {
			t.Parallel()

			storagetest.TestItemMarshalAndUnmarshal(t, tc.test)
		})

		t.Run(fmt.Sprintf("%s clone", tc.name), func(t *testing.T) {
			t.Parallel()

			storagetest.TestItemClone(t, &storagetest.ItemCloneTest{
				Item:    tc.test.Item,
				CmpOpts: tc.test.CmpOpts,
			})
		})
	}
}

func TestUploadItem(t *testing.T) {
	t.Parallel()

	randomAddress := swarm.RandAddress(t)
	randomBatchId := swarm.RandAddress(t).Bytes()

	tests := []struct {
		name string
		test *storagetest.ItemMarshalAndUnmarshalTest
	}{{
		name: "zero values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:       &upload.UploadItem{},
			Factory:    func() storage.Item { return new(upload.UploadItem) },
			MarshalErr: upload.ErrUploadItemMarshalAddressIsZero,
		},
	}, {
		name: "zero address",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &upload.UploadItem{
				Address: swarm.ZeroAddress,
				BatchID: storagetest.MinAddressBytes[:],
			},
			Factory:    func() storage.Item { return new(upload.UploadItem) },
			MarshalErr: upload.ErrUploadItemMarshalAddressIsZero,
		},
	}, {
		name: "nil stamp",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &upload.UploadItem{
				Address: swarm.NewAddress(storagetest.MinAddressBytes[:]),
			},
			Factory:    func() storage.Item { return new(upload.UploadItem) },
			MarshalErr: upload.ErrUploadItemMarshalBatchInvalid,
		},
	}, {
		name: "min values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &upload.UploadItem{
				Address: swarm.NewAddress(storagetest.MinAddressBytes[:]),
				BatchID: storagetest.MinAddressBytes[:],
			},
			Factory: func() storage.Item {
				return &upload.UploadItem{
					Address: swarm.NewAddress(storagetest.MinAddressBytes[:]),
					BatchID: storagetest.MinAddressBytes[:],
				}
			},
		},
	}, {
		name: "max values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &upload.UploadItem{
				Address:  swarm.NewAddress(storagetest.MaxAddressBytes[:]),
				BatchID:  storagetest.MaxAddressBytes[:],
				TagID:    math.MaxUint64,
				Uploaded: math.MaxInt64,
				Synced:   math.MaxInt64,
			},
			Factory: func() storage.Item {
				return &upload.UploadItem{
					Address: swarm.NewAddress(storagetest.MaxAddressBytes[:]),
					BatchID: storagetest.MaxAddressBytes[:],
				}
			},
		},
	}, {
		name: "random values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &upload.UploadItem{
				Address:  randomAddress,
				BatchID:  randomBatchId,
				TagID:    rand.Uint64(),
				Uploaded: rand.Int63(),
				Synced:   rand.Int63(),
			},
			Factory: func() storage.Item {
				return &upload.UploadItem{
					Address: randomAddress,
					BatchID: randomBatchId,
				}
			},
		},
	}, {
		name: "invalid size",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &storagetest.ItemStub{
				MarshalBuf:   []byte{0xFF},
				UnmarshalBuf: []byte{0xFF},
			},
			Factory:      func() storage.Item { return new(upload.UploadItem) },
			UnmarshalErr: upload.ErrUploadItemUnmarshalInvalidSize,
		},
	}}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s marshal/unmarshal", tc.name), func(t *testing.T) {
			t.Parallel()

			storagetest.TestItemMarshalAndUnmarshal(t, tc.test)
		})

		t.Run(fmt.Sprintf("%s clone", tc.name), func(t *testing.T) {
			t.Parallel()

			storagetest.TestItemClone(t, &storagetest.ItemCloneTest{
				Item:    tc.test.Item,
				CmpOpts: tc.test.CmpOpts,
			})
		})
	}
}

func TestItemNextTagID(t *testing.T) {
	t.Parallel()

	zeroValue := upload.NextTagID(0)
	maxValue := upload.NextTagID(math.MaxUint64)

	tests := []struct {
		name string
		test *storagetest.ItemMarshalAndUnmarshalTest
	}{{
		name: "zero values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:    &zeroValue,
			Factory: func() storage.Item { return new(upload.NextTagID) },
		},
	}, {
		name: "max value",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:    &maxValue,
			Factory: func() storage.Item { return new(upload.NextTagID) },
		},
	}, {
		name: "invalid size",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &storagetest.ItemStub{
				MarshalBuf:   []byte{0xFF},
				UnmarshalBuf: []byte{0xFF},
			},
			Factory:      func() storage.Item { return new(upload.NextTagID) },
			UnmarshalErr: upload.ErrNextTagIDUnmarshalInvalidSize,
		},
	}}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s marshal/unmarshal", tc.name), func(t *testing.T) {
			t.Parallel()

			storagetest.TestItemMarshalAndUnmarshal(t, tc.test)
		})

		t.Run(fmt.Sprintf("%s clone", tc.name), func(t *testing.T) {
			t.Parallel()

			storagetest.TestItemClone(t, &storagetest.ItemCloneTest{
				Item:    tc.test.Item,
				CmpOpts: tc.test.CmpOpts,
			})
		})
	}
}

func TestItemDirtyTagItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		test *storagetest.ItemMarshalAndUnmarshalTest
	}{{
		name: "zero values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:    &upload.DirtyTagItem{},
			Factory: func() storage.Item { return new(upload.DirtyTagItem) },
		},
	}, {
		name: "max value",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:    &upload.DirtyTagItem{TagID: math.MaxUint64, Started: math.MaxInt64},
			Factory: func() storage.Item { return new(upload.DirtyTagItem) },
		},
	}, {
		name: "invalid size",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &storagetest.ItemStub{
				MarshalBuf:   []byte{0xFF},
				UnmarshalBuf: []byte{0xFF},
			},
			Factory:      func() storage.Item { return new(upload.DirtyTagItem) },
			UnmarshalErr: upload.ErrDirtyTagItemUnmarshalInvalidSize,
		},
	}}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s marshal/unmarshal", tc.name), func(t *testing.T) {
			t.Parallel()

			storagetest.TestItemMarshalAndUnmarshal(t, tc.test)
		})

		t.Run(fmt.Sprintf("%s clone", tc.name), func(t *testing.T) {
			t.Parallel()

			storagetest.TestItemClone(t, &storagetest.ItemCloneTest{
				Item:    tc.test.Item,
				CmpOpts: tc.test.CmpOpts,
			})
		})
	}
}

func newTestStorage(t *testing.T) transaction.Storage {
	t.Helper()

	storg := internal.NewInmemStorage()
	return storg
}

func TestChunkPutter(t *testing.T) {
	t.Parallel()

	ts := newTestStorage(t)

	tx, done := ts.NewTransaction(context.Background())
	defer done()
	tag, err := upload.NextTag(tx.IndexStore())
	if err != nil {
		t.Fatalf("failed creating tag: %v", err)
	}

	putter, err := upload.NewPutter(tx.IndexStore(), tag.TagID)
	if err != nil {
		t.Fatalf("failed creating putter: %v", err)
	}
	_ = tx.Commit()

	for _, chunk := range chunktest.GenerateTestRandomChunks(10) {
		t.Run(fmt.Sprintf("chunk %s", chunk.Address()), func(t *testing.T) {
			t.Run("put new chunk", func(t *testing.T) {
				err := ts.Run(context.Background(), func(s transaction.Store) error {
					return putter.Put(context.Background(), s, chunk)
				})
				if err != nil {
					t.Fatalf("Put(...): unexpected error: %v", err)
				}
			})

			t.Run("put existing chunk", func(t *testing.T) {
				err := ts.Run(context.Background(), func(s transaction.Store) error {
					return putter.Put(context.Background(), s, chunk)
				})
				if err != nil {
					t.Fatalf("Put(...): unexpected error: %v", err)
				}
			})

			t.Run("verify internal state", func(t *testing.T) {
				ui := &upload.UploadItem{
					Address: chunk.Address(),
					BatchID: chunk.Stamp().BatchID(),
				}
				err := ts.IndexStore().Get(ui)
				if err != nil {
					t.Fatalf("Get(...): unexpected error: %v", err)
				}
				wantUI := &upload.UploadItem{
					Address:  chunk.Address(),
					BatchID:  chunk.Stamp().BatchID(),
					TagID:    tag.TagID,
					Uploaded: now().UnixNano(),
				}

				if diff := cmp.Diff(wantUI, ui); diff != "" {
					t.Fatalf("Get(...): unexpected UploadItem (-want +have):\n%s", diff)
				}

				pi := &upload.PushItem{
					Timestamp: now().UnixNano(),
					Address:   chunk.Address(),
					BatchID:   chunk.Stamp().BatchID(),
				}
				err = ts.IndexStore().Get(pi)
				if err != nil {
					t.Fatalf("Get(...): unexpected error: %v", err)
				}
				wantPI := &upload.PushItem{
					Address:   chunk.Address(),
					BatchID:   chunk.Stamp().BatchID(),
					TagID:     tag.TagID,
					Timestamp: now().UnixNano(),
				}

				if diff := cmp.Diff(wantPI, pi); diff != "" {
					t.Fatalf("Get(...): unexpected UploadItem (-want +have):\n%s", diff)
				}

				have, err := ts.ChunkStore().Get(context.Background(), chunk.Address())
				if err != nil {
					t.Fatalf("Get(...): unexpected error: %v", err)
				}
				if want := chunk; !want.Equal(have) {
					t.Fatalf("Get(...): chunk mismatch:\nwant: %x\nhave: %x", want, have)
				}
			})
		})
	}

	t.Run("iterate all", func(t *testing.T) {
		count := 0
		err := ts.IndexStore().Iterate(
			storage.Query{
				Factory: func() storage.Item { return new(upload.UploadItem) },
			},
			func(r storage.Result) (bool, error) {
				address := swarm.NewAddress([]byte(r.ID[:32]))
				synced := r.Entry.(*upload.UploadItem).Synced != 0
				count++
				if synced {
					t.Fatal("expected synced to be false")
				}
				has, err := ts.ChunkStore().Has(context.Background(), address)
				if err != nil {
					t.Fatalf("unexpected error in Has(...): %v", err)
				}
				if !has {
					t.Fatalf("expected chunk to be present %s", address.String())
				}
				return false, nil
			},
		)
		if err != nil {
			t.Fatalf("IterateAll(...): unexpected error %v", err)
		}
		if count != 10 {
			t.Fatalf("unexpected count: want 10 have %d", count)
		}
	})

	t.Run("close with reference", func(t *testing.T) {
		addr := swarm.RandAddress(t)

		err := ts.Run(context.Background(), func(s transaction.Store) error {
			return putter.Close(s.IndexStore(), addr)
		})
		if err != nil {
			t.Fatalf("Close(...): unexpected error %v", err)
		}

		var ti upload.TagItem

		err = ts.Run(context.Background(), func(s transaction.Store) error {
			ti, err = upload.TagInfo(s.IndexStore(), tag.TagID)
			return err
		})
		if err != nil {
			t.Fatalf("TagInfo(...): unexpected error %v", err)
		}

		wantTI := upload.TagItem{
			TagID:     tag.TagID,
			Split:     20,
			Seen:      10,
			StartedAt: now().UnixNano(),
			Address:   addr,
		}
		if diff := cmp.Diff(wantTI, ti); diff != "" {
			t.Fatalf("Get(...): unexpected TagItem (-want +have):\n%s", diff)
		}

		t.Run("iterate all tag items", func(t *testing.T) {
			var tagItemsCount, uploaded, synced uint64
			err := upload.IterateAllTagItems(ts.IndexStore(), func(ti *upload.TagItem) (bool, error) {
				uploaded += ti.Split
				synced += ti.Synced
				tagItemsCount++
				return false, nil
			})
			if err != nil {
				t.Fatalf("IterateAllTagItems(...): unexpected error %v", err)
			}
			if tagItemsCount != 1 {
				t.Fatalf("unexpected tagItemsCount: want 1 have %d", tagItemsCount)
			}
			if uploaded != 20 {
				t.Fatalf("unexpected uploaded: want 20 have %d", uploaded)
			}
			if synced != 0 {
				t.Fatalf("unexpected synced: want 0 have %d", synced)
			}
		})
	})

	t.Run("error after close", func(t *testing.T) {
		err := ts.Run(context.Background(), func(s transaction.Store) error {
			return putter.Put(context.Background(), s, chunktest.GenerateTestRandomChunk())
		})
		if !errors.Is(err, upload.ErrPutterAlreadyClosed) {
			t.Fatalf("unexpected error, expected: %v, got: %v", upload.ErrPutterAlreadyClosed, err)
		}
	})

	t.Run("restart putter", func(t *testing.T) {
		var putter internal.PutterCloserWithReference

		err = ts.Run(context.Background(), func(s transaction.Store) error {
			putter, err = upload.NewPutter(s.IndexStore(), tag.TagID)
			return err
		})
		if err != nil {
			t.Fatalf("failed creating putter: %v", err)
		}

		for _, chunk := range chunktest.GenerateTestRandomChunks(5) {
			if err := ts.Run(context.Background(), func(s transaction.Store) error {
				return putter.Put(context.Background(), s, chunk)
			}); err != nil {
				t.Fatalf("Put(...): unexpected error: %v", err)
			}
		}

		// close with different address
		addr := swarm.RandAddress(t)
		if err := ts.Run(context.Background(), func(s transaction.Store) error {
			return putter.Close(s.IndexStore(), addr)
		}); err != nil {
			t.Fatalf("Close(...): unexpected error %v", err)
		}

		ti, err := upload.TagInfo(ts.IndexStore(), tag.TagID)
		if err != nil {
			t.Fatalf("TagInfo(...): unexpected error %v", err)
		}

		wantTI := upload.TagItem{
			TagID:     tag.TagID,
			Split:     25,
			Seen:      10,
			StartedAt: now().UnixNano(),
			Address:   addr,
		}

		if diff := cmp.Diff(wantTI, ti); diff != "" {
			t.Fatalf("Get(...): unexpected TagItem (-want +have):\n%s", diff)
		}
	})
}

func TestChunkReporter(t *testing.T) {
	t.Parallel()

	ts := newTestStorage(t)

	var (
		tag    upload.TagItem
		putter internal.PutterCloserWithReference
		err    error
	)
	if err := ts.Run(context.Background(), func(s transaction.Store) error {
		tag, err = upload.NextTag(s.IndexStore())
		return err
	}); err != nil {
		t.Fatalf("failed creating tag: %v", err)
	}

	if err := ts.Run(context.Background(), func(s transaction.Store) error {
		putter, err = upload.NewPutter(s.IndexStore(), tag.TagID)
		return err
	}); err != nil {
		t.Fatalf("failed creating putter: %v", err)
	}

	for idx, chunk := range chunktest.GenerateTestRandomChunks(10) {
		t.Run(fmt.Sprintf("chunk %s", chunk.Address()), func(t *testing.T) {
			if err := ts.Run(context.Background(), func(s transaction.Store) error {
				return putter.Put(context.Background(), s, chunk)
			}); err != nil {
				t.Fatalf("Put(...): unexpected error: %v", err)
			}

			report := func(ch swarm.Chunk, state int) {
				t.Helper()
				if err := ts.Run(context.Background(), func(s transaction.Store) error {
					return upload.Report(context.Background(), s, ch, state)
				}); err != nil {
					t.Fatalf("Report(...): unexpected error: %v", err)
				}
			}

			t.Run("mark sent", func(t *testing.T) {
				report(chunk, storage.ChunkSent)
			})

			if idx < 4 {
				t.Run("mark stored", func(t *testing.T) {
					report(chunk, storage.ChunkStored)
				})
			}

			if idx >= 4 && idx < 8 {
				t.Run("mark synced", func(t *testing.T) {
					report(chunk, storage.ChunkSynced)
				})
			}

			if idx >= 8 {
				t.Run("mark could not sync", func(t *testing.T) {
					report(chunk, storage.ChunkCouldNotSync)
				})
			}

			t.Run("verify internal state", func(t *testing.T) {
				ti := &upload.TagItem{
					TagID: tag.TagID,
				}
				err := ts.IndexStore().Get(ti)
				if err != nil {
					t.Fatalf("Get(...): unexpected error: %v", err)
				}
				var synced, sent, stored uint64
				sent = uint64(idx + 1)
				if idx >= 8 {
					synced, stored = 8, 4
				} else {
					synced, stored = uint64(idx+1), uint64(idx+1)
					if idx >= 4 {
						stored = 4
					}
				}
				wantTI := &upload.TagItem{
					TagID:     tag.TagID,
					StartedAt: now().UnixNano(),
					Sent:      sent,
					Synced:    synced,
					Stored:    stored,
				}

				if diff := cmp.Diff(wantTI, ti); diff != "" {
					t.Fatalf("Get(...): unexpected TagItem (-want +have):\n%s", diff)
				}

				ui := &upload.UploadItem{
					Address: chunk.Address(),
					BatchID: chunk.Stamp().BatchID(),
				}
				has, err := ts.IndexStore().Has(ui)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if has {
					t.Fatalf("expected to not be found: %s", ui)
				}

				pi := &upload.PushItem{
					Timestamp: now().UnixNano(),
					Address:   chunk.Address(),
					BatchID:   chunk.Stamp().BatchID(),
				}
				has, err = ts.IndexStore().Has(pi)
				if err != nil {
					t.Fatalf("Has(...): unexpected error: %v", err)
				}
				if has {
					t.Fatalf("Has(...): expected to not be found: %s", pi)
				}

				have, err := ts.ChunkStore().Has(context.Background(), chunk.Address())
				if err != nil {
					t.Fatalf("Get(...): unexpected error: %v", err)
				}
				if have {
					t.Fatalf("Get(...): chunk expected to not be found: %s", chunk.Address())
				}
			})
		})
	}

	t.Run("close with reference", func(t *testing.T) {
		addr := swarm.RandAddress(t)

		err := ts.Run(context.Background(), func(s transaction.Store) error { return putter.Close(s.IndexStore(), addr) })
		if err != nil {
			t.Fatalf("Close(...): unexpected error %v", err)
		}

		var ti upload.TagItem
		err = ts.Run(context.Background(), func(s transaction.Store) error {
			ti, err = upload.TagInfo(s.IndexStore(), tag.TagID)
			return err
		})
		if err != nil {
			t.Fatalf("TagInfo(...): unexpected error %v", err)
		}

		wantTI := upload.TagItem{
			TagID:     tag.TagID,
			Split:     10,
			Seen:      0,
			Stored:    4,
			Sent:      10,
			Synced:    8,
			StartedAt: now().UnixNano(),
			Address:   addr,
		}
		if diff := cmp.Diff(wantTI, ti); diff != "" {
			t.Fatalf("Get(...): unexpected TagItem (-want +have):\n%s", diff)
		}
	})
}

func TestNextTagID(t *testing.T) {
	t.Parallel()

	ts := newTestStorage(t)

	for i := 1; i < 4; i++ {
		var tag upload.TagItem
		var err error
		err = ts.Run(context.Background(), func(s transaction.Store) error {
			tag, err = upload.NextTag(s.IndexStore())
			return err
		})
		if err != nil {
			t.Fatalf("failed creating tag: %v", err)
		}

		if tag.TagID != uint64(i) {
			t.Fatalf("incorrect tag ID returned, exp: %d found %d", i, tag.TagID)
		}
	}

	var lastTag upload.NextTagID
	err := ts.IndexStore().Get(&lastTag)
	if err != nil {
		t.Fatal(err)
	}

	if uint64(lastTag) != 3 {
		t.Fatalf("incorrect value for last tag, exp 3 found %d", uint64(lastTag))
	}
}

func TestListTags(t *testing.T) {
	t.Parallel()

	ts := newTestStorage(t)

	want := make([]upload.TagItem, 10)
	for i := range want {
		var tag upload.TagItem
		var err error
		err = ts.Run(context.Background(), func(s transaction.Store) error {
			tag, err = upload.NextTag(s.IndexStore())
			return err
		})
		if err != nil {
			t.Fatalf("failed creating tag: %v", err)
		}
		want[i] = tag
	}

	have, err := upload.ListAllTags(ts.IndexStore())
	if err != nil {
		t.Fatalf("upload.ListAllTags(): unexpected error: %v", err)
	}

	opts := cmpopts.SortSlices(func(i, j upload.TagItem) bool { return i.TagID < j.TagID })
	if diff := cmp.Diff(want, have, opts); diff != "" {
		t.Fatalf("upload.ListAllTags(): mismatch (-want +have):\n%s", diff)
	}
}

func TestIterate(t *testing.T) {
	t.Parallel()

	ts := newTestStorage(t)

	t.Run("on empty storage does not call the callback fn", func(t *testing.T) {
		err := upload.IteratePending(context.Background(), ts, func(chunk swarm.Chunk) (bool, error) {
			t.Fatal("unexpected call")
			return false, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("iterates chunks", func(t *testing.T) {
		var tag upload.TagItem
		var err error
		err = ts.Run(context.Background(), func(s transaction.Store) error {
			tag, err = upload.NextTag(s.IndexStore())
			return err
		})
		if err != nil {
			t.Fatalf("failed creating tag: %v", err)
		}

		var putter internal.PutterCloserWithReference
		err = ts.Run(context.Background(), func(s transaction.Store) error {
			putter, err = upload.NewPutter(s.IndexStore(), tag.TagID)
			return err
		})
		if err != nil {
			t.Fatalf("failed creating putter: %v", err)
		}

		chunk1, chunk2 := chunktest.GenerateTestRandomChunk(), chunktest.GenerateTestRandomChunk()
		err = put(t, ts, putter, chunk1)
		if err != nil {
			t.Fatalf("session.Put(...): unexpected error: %v", err)
		}
		err = put(t, ts, putter, chunk2)
		if err != nil {
			t.Fatalf("session.Put(...): unexpected error: %v", err)
		}

		var count int

		err = upload.IteratePending(context.Background(), ts, func(chunk swarm.Chunk) (bool, error) {
			count++
			if !chunk.Equal(chunk1) && !chunk.Equal(chunk2) {
				return true, fmt.Errorf("unknown chunk %s", chunk.Address())
			}
			return false, nil
		})
		if err != nil {
			t.Fatalf("Iterate(...): unexpected error: %v", err)
		}

		if count != 0 {
			t.Fatalf("expected to iterate 0 chunks, got: %v", count)
		}

		err = ts.Run(context.Background(), func(s transaction.Store) error { return putter.Close(s.IndexStore(), swarm.ZeroAddress) })
		if err != nil {
			t.Fatalf("Close(...) error: %v", err)
		}

		err = upload.IteratePending(context.Background(), ts, func(chunk swarm.Chunk) (bool, error) {
			count++
			if !chunk.Equal(chunk1) && !chunk.Equal(chunk2) {
				return true, fmt.Errorf("unknown chunk %s", chunk.Address())
			}
			return false, nil
		})
		if err != nil {
			t.Fatalf("Iterate(...): unexpected error: %v", err)
		}

		if count != 2 {
			t.Fatalf("expected to iterate two chunks, got: %v", count)
		}
	})
}

func TestDeleteTag(t *testing.T) {
	t.Parallel()

	ts := newTestStorage(t)

	var tag upload.TagItem
	var err error
	err = ts.Run(context.Background(), func(s transaction.Store) error {
		tag, err = upload.NextTag(s.IndexStore())
		return err
	})
	if err != nil {
		t.Fatalf("failed creating tag: %v", err)
	}

	err = ts.Run(context.Background(), func(s transaction.Store) error {
		return upload.DeleteTag(s.IndexStore(), tag.TagID)
	})
	if err != nil {
		t.Fatalf("upload.DeleteTag(): unexpected error: %v", err)
	}

	_, err = upload.TagInfo(ts.IndexStore(), tag.TagID)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("want: %v; have: %v", storage.ErrNotFound, err)
	}
}

func TestBatchIDForChunk(t *testing.T) {
	t.Parallel()

	ts := newTestStorage(t)

	var tag upload.TagItem
	var err error
	err = ts.Run(context.Background(), func(s transaction.Store) error {
		tag, err = upload.NextTag(s.IndexStore())
		return err
	})
	if err != nil {
		t.Fatalf("failed creating tag: %v", err)
	}

	var putter internal.PutterCloserWithReference
	err = ts.Run(context.Background(), func(s transaction.Store) error {
		putter, err = upload.NewPutter(s.IndexStore(), tag.TagID)
		return err
	})
	if err != nil {
		t.Fatalf("failed creating putter: %v", err)
	}

	chunk := chunktest.GenerateTestRandomChunk()
	if err := put(t, ts, putter, chunk); err != nil {
		t.Fatalf("Put(...): unexpected error: %v", err)
	}

	batchID, err := upload.BatchIDForChunk(ts.IndexStore(), chunk.Address())
	if err != nil {
		t.Fatalf("BatchIDForChunk(...): unexpected error: %v", err)
	}

	if !bytes.Equal(batchID, chunk.Stamp().BatchID()) {
		t.Fatalf("BatchIDForChunk(...): want %x; got %x", chunk.Stamp().BatchID(), batchID)
	}
}

func TestCleanup(t *testing.T) {
	t.Parallel()

	t.Run("cleanup putter", func(t *testing.T) {
		t.Parallel()

		ts := newTestStorage(t)

		var tag upload.TagItem
		var err error
		err = ts.Run(context.Background(), func(s transaction.Store) error {
			tag, err = upload.NextTag(s.IndexStore())
			return err
		})
		if err != nil {
			t.Fatalf("failed creating tag: %v", err)
		}

		var putter internal.PutterCloserWithReference
		err = ts.Run(context.Background(), func(s transaction.Store) error {
			putter, err = upload.NewPutter(s.IndexStore(), tag.TagID)
			return err
		})
		if err != nil {
			t.Fatalf("failed creating putter: %v", err)
		}

		chunk := chunktest.GenerateTestRandomChunk()
		err = put(t, ts, putter, chunk)
		if err != nil {
			t.Fatal("session.Put(...): unexpected error", err)
		}

		err = putter.Cleanup(ts)
		if err != nil {
			t.Fatal("upload.Cleanup(...): unexpected error", err)
		}

		count := 0
		_ = upload.IteratePending(context.Background(), ts, func(chunk swarm.Chunk) (bool, error) {
			count++
			return false, nil
		})
		if count != 0 {
			t.Fatalf("expected to iterate 0 chunks, got: %v", count)
		}

		if _, err := ts.ChunkStore().Get(context.Background(), chunk.Address()); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("expected chunk not found error, got: %v", err)
		}
	})

	t.Run("cleanup dirty", func(t *testing.T) {
		t.Parallel()

		ts := newTestStorage(t)

		var tag upload.TagItem
		var err error
		err = ts.Run(context.Background(), func(s transaction.Store) error {
			tag, err = upload.NextTag(s.IndexStore())
			return err
		})
		if err != nil {
			t.Fatalf("failed creating tag: %v", err)
		}

		var putter internal.PutterCloserWithReference
		err = ts.Run(context.Background(), func(s transaction.Store) error {
			putter, err = upload.NewPutter(s.IndexStore(), tag.TagID)
			return err
		})
		if err != nil {
			t.Fatalf("failed creating putter: %v", err)
		}

		chunk := chunktest.GenerateTestRandomChunk()
		err = put(t, ts, putter, chunk)
		if err != nil {
			t.Fatal("session.Put(...): unexpected error", err)
		}

		err = upload.CleanupDirty(ts)
		if err != nil {
			t.Fatal("upload.Cleanup(...): unexpected error", err)
		}

		count := 0
		_ = upload.IteratePending(context.Background(), ts, func(chunk swarm.Chunk) (bool, error) {
			count++
			return false, nil
		})
		if count != 0 {
			t.Fatalf("expected to iterate 0 chunks, got: %v", count)
		}

		if _, err := ts.ChunkStore().Get(context.Background(), chunk.Address()); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("expected chunk not found error, got: %v", err)
		}
	})
}

func put(t *testing.T, ts transaction.Storage, putter internal.PutterCloserWithReference, ch swarm.Chunk) error {
	t.Helper()
	return ts.Run(context.Background(), func(s transaction.Store) error {
		return putter.Put(context.Background(), s, ch)
	})
}
