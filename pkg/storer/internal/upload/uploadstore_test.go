// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package upload_test

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/ethersphere/bee/pkg/postage"
	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/storagetest"
	chunktest "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/storer/internal"
	"github.com/ethersphere/bee/pkg/storer/internal/upload"
	"github.com/ethersphere/bee/pkg/swarm"
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
		tc := tc

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
		tc := tc

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
		tc := tc

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
		tc := tc

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

func newTestStorage(t *testing.T) internal.Storage {
	t.Helper()

	storg, closer := internal.NewInmemStorage()
	t.Cleanup(func() {
		err := closer()
		if err != nil {
			t.Errorf("failed closing storage: %v", err)
		}
	})

	return storg
}

func TestChunkPutter(t *testing.T) {
	t.Parallel()

	ts := newTestStorage(t)

	tag, err := upload.NextTag(ts.IndexStore())
	if err != nil {
		t.Fatalf("failed creating tag: %v", err)
	}

	putter, err := upload.NewPutter(ts, tag.TagID)
	if err != nil {
		t.Fatalf("failed creating putter: %v", err)
	}

	for _, chunk := range chunktest.GenerateTestRandomChunks(10) {
		t.Run(fmt.Sprintf("chunk %s", chunk.Address()), func(t *testing.T) {
			t.Run("put new chunk", func(t *testing.T) {
				err := putter.Put(context.Background(), chunk)
				if err != nil {
					t.Fatalf("Put(...): unexpected error: %v", err)
				}
			})

			t.Run("put existing chunk", func(t *testing.T) {
				err := putter.Put(context.Background(), chunk)
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
					t.Fatalf("Get(...): chunk missmatch:\nwant: %x\nhave: %x", want, have)
				}
			})
		})
	}

	t.Run("iterate all", func(t *testing.T) {
		count := 0
		err := upload.IterateAll(ts.IndexStore(), func(addr swarm.Address, synced bool) (bool, error) {
			count++
			if synced {
				t.Fatal("expected synced to be false")
			}
			has, err := ts.ChunkStore().Has(context.Background(), addr)
			if err != nil {
				t.Fatalf("unexpected error in Has(...): %v", err)
			}
			if !has {
				t.Fatalf("expected chunk to be present %s", addr.String())
			}
			return false, nil
		})
		if err != nil {
			t.Fatalf("IterateAll(...): unexpected error %v", err)
		}
		if count != 10 {
			t.Fatalf("unexpected count: want 10 have %d", count)
		}
	})

	t.Run("close with reference", func(t *testing.T) {
		addr := swarm.RandAddress(t)

		err := putter.Close(addr)
		if err != nil {
			t.Fatalf("Close(...): unexpected error %v", err)
		}

		ti, err := upload.TagInfo(ts.IndexStore(), tag.TagID)
		if err != nil {
			t.Fatalf("TagInfo(...): unexpected error %v", err)
		}

		wantTI := upload.TagItem{
			TagID:     tag.TagID,
			Split:     20,
			Seen:      10,
			StartedAt: now().Unix(),
			Address:   addr,
		}
		if diff := cmp.Diff(wantTI, ti); diff != "" {
			t.Fatalf("Get(...): unexpected TagItem (-want +have):\n%s", diff)
		}
	})

	t.Run("error after close", func(t *testing.T) {
		err := putter.Put(context.Background(), chunktest.GenerateTestRandomChunk())
		if !errors.Is(err, upload.ErrPutterAlreadyClosed) {
			t.Fatalf("unexpected error, expected: %v, got: %v", upload.ErrPutterAlreadyClosed, err)
		}
	})
}

func TestChunkReporter(t *testing.T) {
	t.Parallel()

	ts := newTestStorage(t)

	tag, err := upload.NextTag(ts.IndexStore())
	if err != nil {
		t.Fatalf("failed creating tag: %v", err)
	}

	putter, err := upload.NewPutter(ts, tag.TagID)
	if err != nil {
		t.Fatalf("failed creating putter: %v", err)
	}

	reporter := upload.NewPushReporter(ts)

	for idx, chunk := range chunktest.GenerateTestRandomChunks(10) {
		t.Run(fmt.Sprintf("chunk %s", chunk.Address()), func(t *testing.T) {
			err := putter.Put(context.Background(), chunk)
			if err != nil {
				t.Fatalf("Put(...): unexpected error: %v", err)
			}

			t.Run("mark sent", func(t *testing.T) {
				err := reporter.Report(context.Background(), chunk, storage.ChunkSent)
				if err != nil {
					t.Fatalf("Report(...): unexpected error: %v", err)
				}
			})

			if idx < 4 {
				t.Run("mark stored", func(t *testing.T) {
					err := reporter.Report(context.Background(), chunk, storage.ChunkStored)
					if err != nil {
						t.Fatalf("Report(...): unexpected error: %v", err)
					}
				})
			}

			if idx >= 4 && idx < 8 {
				t.Run("mark synced", func(t *testing.T) {
					err := reporter.Report(context.Background(), chunk, storage.ChunkSynced)
					if err != nil {
						t.Fatalf("Report(...): unexpected error: %v", err)
					}
				})
			}

			if idx >= 8 {
				t.Run("mark could not sync", func(t *testing.T) {
					err := reporter.Report(context.Background(), chunk, storage.ChunkCouldNotSync)
					if err != nil {
						t.Fatalf("Report(...): unexpected error: %v", err)
					}
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
					StartedAt: now().Unix(),
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
				err = ts.IndexStore().Get(ui)
				if err != nil {
					t.Fatalf("Get(...): unexpected error: %v", err)
				}
				wantUI := &upload.UploadItem{
					Address:  chunk.Address(),
					BatchID:  chunk.Stamp().BatchID(),
					TagID:    tag.TagID,
					Uploaded: now().UnixNano(),
					Synced:   now().Unix(),
				}

				if diff := cmp.Diff(wantUI, ui); diff != "" {
					t.Fatalf("Get(...): unexpected UploadItem (-want +have):\n%s", diff)
				}

				pi := &upload.PushItem{
					Timestamp: now().Unix(),
					Address:   chunk.Address(),
					BatchID:   chunk.Stamp().BatchID(),
				}
				has, err := ts.IndexStore().Has(pi)
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

		err := putter.Close(addr)
		if err != nil {
			t.Fatalf("Close(...): unexpected error %v", err)
		}

		ti, err := upload.TagInfo(ts.IndexStore(), tag.TagID)
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
			StartedAt: now().Unix(),
			Address:   addr,
		}
		if diff := cmp.Diff(wantTI, ti); diff != "" {
			t.Fatalf("Get(...): unexpected TagItem (-want +have):\n%s", diff)
		}
	})
}

func TestStampIndexHandling(t *testing.T) {
	t.Parallel()

	ts := newTestStorage(t)

	tag, err := upload.NextTag(ts.IndexStore())
	if err != nil {
		t.Fatalf("failed creating tag: %v", err)
	}

	putter, err := upload.NewPutter(ts, tag.TagID)
	if err != nil {
		t.Fatalf("failed creating putter: %v", err)
	}

	t.Run("put chunk with immutable batch", func(t *testing.T) {
		chunk := chunktest.GenerateTestRandomChunk()
		chunk = chunk.WithBatch(
			chunk.Radius(),
			chunk.Depth(),
			chunk.BucketDepth(),
			true,
		)
		if err := putter.Put(context.Background(), chunk); err != nil {
			t.Fatalf("Put(...): unexpected error: %v", err)
		}

		chunk2 := chunktest.GenerateTestRandomChunk().WithStamp(chunk.Stamp())

		want := upload.ErrOverwriteOfImmutableBatch
		have := putter.Put(context.Background(), chunk2)
		if !errors.Is(have, want) {
			t.Fatalf("Put(...): unexpected error:\n\twant: %v\n\thave: %v", want, have)
		}
	})

	t.Run("put existing index with older batch timestamp", func(t *testing.T) {
		chunk := chunktest.GenerateTestRandomChunk()
		if err := putter.Put(context.Background(), chunk); err != nil {
			t.Fatalf("Put(...): unexpected error: %v", err)
		}

		decTS := binary.BigEndian.Uint64(chunk.Stamp().Timestamp())
		encTS := make([]byte, 8)
		binary.BigEndian.PutUint64(encTS, decTS-1)

		stamp := postage.NewStamp(
			chunk.Stamp().BatchID(),
			chunk.Stamp().Index(),
			encTS,
			chunk.Stamp().Sig(),
		)

		chunk2 := chunktest.GenerateTestRandomChunk().WithStamp(stamp)

		want := upload.ErrOverwriteOfNewerBatch
		have := putter.Put(context.Background(), chunk2)
		if !errors.Is(have, want) {
			t.Fatalf("Put(...): unexpected error:\n\twant: %v\n\thave: %v", want, have)
		}
	})

	t.Run("put existing chunk with newer batch timestamp", func(t *testing.T) {
		chunk := chunktest.GenerateTestRandomChunk()
		if err := putter.Put(context.Background(), chunk); err != nil {
			t.Fatalf("Put(...): unexpected error: %v", err)
		}

		decTS := binary.BigEndian.Uint64(chunk.Stamp().Timestamp())
		encTS := make([]byte, 8)
		binary.BigEndian.PutUint64(encTS, decTS+1)

		stamp := postage.NewStamp(
			chunk.Stamp().BatchID(),
			chunk.Stamp().Index(),
			encTS,
			chunk.Stamp().Sig(),
		)

		chunk2 := chunktest.GenerateTestRandomChunk().WithStamp(stamp)

		if err := putter.Put(context.Background(), chunk2); err != nil {
			t.Fatalf("Put(...): unexpected error: %v", err)
		}
	})
}

func TestNextTagID(t *testing.T) {
	t.Parallel()

	ts := newTestStorage(t)

	for i := 1; i < 4; i++ {
		tag, err := upload.NextTag(ts.IndexStore())
		if err != nil {
			t.Fatal(err)
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
		ti, err := upload.NextTag(ts.IndexStore())
		if err != nil {
			t.Fatalf("failed creating tag: %v", err)
		}
		want[i] = ti
	}

	have, err := upload.ListAllTags(ts.IndexStore())
	if err != nil {
		t.Fatalf("upload.ListAllTags(): unexpected error: %v", err)
	}

	opts := cmpopts.SortSlices(func(i, j upload.TagItem) bool { return i.TagID < j.TagID })
	if diff := cmp.Diff(want, have, opts); diff != "" {
		t.Fatalf("upload.ListAllTags(): missmatch (-want +have):\n%s", diff)
	}
}

func TestIterate(t *testing.T) {
	t.Parallel()

	ts := newTestStorage(t)

	t.Run("on empty storage does not call the callback fn", func(t *testing.T) {
		err := upload.Iterate(context.Background(), ts, func(chunk swarm.Chunk) (bool, error) {
			t.Fatal("unexpected call")
			return false, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("iterates chunks", func(t *testing.T) {
		tag, err := upload.NextTag(ts.IndexStore())
		if err != nil {
			t.Fatalf("failed creating tag: %v", err)
		}

		putter, err := upload.NewPutter(ts, tag.TagID)
		if err != nil {
			t.Fatalf("failed creating putter: %v", err)
		}

		chunk1, chunk2 := chunktest.GenerateTestRandomChunk(), chunktest.GenerateTestRandomChunk()
		err = putter.Put(context.Background(), chunk1)
		if err != nil {
			t.Fatalf("session.Put(...): unexpected error: %v", err)
		}
		err = putter.Put(context.Background(), chunk2)
		if err != nil {
			t.Fatalf("session.Put(...): unexpected error: %v", err)
		}

		var count int

		err = upload.Iterate(context.Background(), ts, func(chunk swarm.Chunk) (bool, error) {
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

	tag, err := upload.NextTag(ts.IndexStore())
	if err != nil {
		t.Fatal("failed creating tag", err)
	}

	err = upload.DeleteTag(ts.IndexStore(), tag.TagID)
	if err != nil {
		t.Fatalf("upload.DeleteTag(): unexpected error: %v", err)
	}

	_, err = upload.TagInfo(ts.IndexStore(), tag.TagID)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("want: %v; have: %v", storage.ErrNotFound, err)
	}
}
