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

	"github.com/ethersphere/bee/pkg/localstorev2/internal"
	"github.com/ethersphere/bee/pkg/localstorev2/internal/upload"
	"github.com/ethersphere/bee/pkg/postage"
	chunktest "github.com/ethersphere/bee/pkg/storage/testing"
	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/storagetest"
	"github.com/ethersphere/bee/pkg/swarm"
	swarmtesting "github.com/ethersphere/bee/pkg/swarm/test"
	"github.com/google/go-cmp/cmp"
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
				Address:   swarmtesting.RandomAddress(),
				BatchID:   swarmtesting.RandomAddress().Bytes(),
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
				Address:   swarmtesting.RandomAddress(),
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

	randomAddress := swarmtesting.RandomAddress()
	randomBatchId := swarmtesting.RandomAddress().Bytes()

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

	const tagID = 1
	putter, err := upload.NewPutter(ts, tagID)
	if err != nil {
		t.Fatalf("failed creating putter: %v", err)
	}

	for _, chunk := range chunktest.GenerateTestRandomChunks(10) {
		t.Run(fmt.Sprintf("chunk %s", chunk.Address()), func(t *testing.T) {
			t.Run("put new chunk", func(t *testing.T) {
				err := putter.Put(context.TODO(), chunk)
				if err != nil {
					t.Fatalf("Put(...): unexpected error: %v", err)
				}
			})

			t.Run("put existing chunk", func(t *testing.T) {
				err := putter.Put(context.TODO(), chunk)
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
					TagID:    tagID,
					Uploaded: now().Unix(),
				}

				if diff := cmp.Diff(wantUI, ui); diff != "" {
					t.Fatalf("Get(...): unexpected UploadItem (-want +have):\n%s", diff)
				}

				pi := &upload.PushItem{
					Timestamp: now().Unix(),
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
					TagID:     tagID,
					Timestamp: now().Unix(),
				}

				if diff := cmp.Diff(wantPI, pi); diff != "" {
					t.Fatalf("Get(...): unexpected UploadItem (-want +have):\n%s", diff)
				}

				have, err := ts.ChunkStore().Get(context.TODO(), chunk.Address())
				if err != nil {
					t.Fatalf("Get(...): unexpected error: %v", err)
				}
				if want := chunk; !want.Equal(have) {
					t.Fatalf("Get(...): chunk missmatch:\nwant: %x\nhave: %x", want, have)
				}
			})
		})
	}

	t.Run("close with reference", func(t *testing.T) {
		addr := swarmtesting.RandomAddress()

		err := putter.Close(addr)
		if err != nil {
			t.Fatalf("Close(...): unexpected error %v", err)
		}

		ti, err := upload.GetTagInfo(ts.IndexStore(), tagID)
		if err != nil {
			t.Fatalf("GetTagInfo(...): unexpected error %v", err)
		}

		wantTI := upload.TagItem{
			TagID:     tagID,
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
		err := putter.Put(context.TODO(), chunktest.GenerateTestRandomChunk())
		if !errors.Is(err, upload.ErrPutterAlreadyClosed) {
			t.Fatalf("unexpected error, expected: %v, got: %v", upload.ErrPutterAlreadyClosed, err)
		}
	})
}

func TestChunkReporter(t *testing.T) {
	t.Parallel()

	ts := newTestStorage(t)

	const tagID = 1
	putter, err := upload.NewPutter(ts, tagID)
	if err != nil {
		t.Fatalf("failed creating putter: %v", err)
	}

	reporter := upload.NewPushReporter(ts)

	for idx, chunk := range chunktest.GenerateTestRandomChunks(10) {
		t.Run(fmt.Sprintf("chunk %s", chunk.Address()), func(t *testing.T) {
			err := putter.Put(context.TODO(), chunk)
			if err != nil {
				t.Fatalf("Put(...): unexpected error: %v", err)
			}

			t.Run("mark sent", func(t *testing.T) {
				err := reporter.Report(context.TODO(), chunk, storage.ChunkSent)
				if err != nil {
					t.Fatalf("Report(...): unexpected error: %v", err)
				}
			})

			t.Run("mark stored", func(t *testing.T) {
				err := reporter.Report(context.TODO(), chunk, storage.ChunkStored)
				if err != nil {
					t.Fatalf("Report(...): unexpected error: %v", err)
				}
			})

			t.Run("mark synced", func(t *testing.T) {
				err := reporter.Report(context.TODO(), chunk, storage.ChunkSynced)
				if err != nil {
					t.Fatalf("Report(...): unexpected error: %v", err)
				}
			})

			t.Run("verify internal state", func(t *testing.T) {
				ti := &upload.TagItem{
					TagID: tagID,
				}
				err := ts.IndexStore().Get(ti)
				if err != nil {
					t.Fatalf("Get(...): unexpected error: %v", err)
				}
				count := uint64(idx + 1)
				wantTI := &upload.TagItem{
					TagID:     tagID,
					StartedAt: now().Unix(),
					Sent:      count,
					Synced:    count,
					Stored:    count,
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
					TagID:    tagID,
					Uploaded: now().Unix(),
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

				have, err := ts.ChunkStore().Has(context.TODO(), chunk.Address())
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
		addr := swarmtesting.RandomAddress()

		err := putter.Close(addr)
		if err != nil {
			t.Fatalf("Close(...): unexpected error %v", err)
		}

		ti, err := upload.GetTagInfo(ts.IndexStore(), tagID)
		if err != nil {
			t.Fatalf("GetTagInfo(...): unexpected error %v", err)
		}

		wantTI := upload.TagItem{
			TagID:     tagID,
			Split:     10,
			Seen:      0,
			Stored:    10,
			Sent:      10,
			Synced:    10,
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

	const tagID = 1
	putter, err := upload.NewPutter(ts, tagID)
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
		if err := putter.Put(context.TODO(), chunk); err != nil {
			t.Fatalf("Put(...): unexpected error: %v", err)
		}

		chunk2 := chunktest.GenerateTestRandomChunk().WithStamp(chunk.Stamp())

		want := upload.ErrOverwriteOfImmutableBatch
		have := putter.Put(context.TODO(), chunk2)
		if !errors.Is(have, want) {
			t.Fatalf("Put(...): unexpected error:\n\twant: %v\n\thave: %v", want, have)
		}
	})

	t.Run("put existing index with older batch timestamp", func(t *testing.T) {
		chunk := chunktest.GenerateTestRandomChunk()
		if err := putter.Put(context.TODO(), chunk); err != nil {
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
		have := putter.Put(context.TODO(), chunk2)
		if !errors.Is(have, want) {
			t.Fatalf("Put(...): unexpected error:\n\twant: %v\n\thave: %v", want, have)
		}
	})

	t.Run("put existing chunk with newer batch timestamp", func(t *testing.T) {
		chunk := chunktest.GenerateTestRandomChunk()
		if err := putter.Put(context.TODO(), chunk); err != nil {
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

		if err := putter.Put(context.TODO(), chunk2); err != nil {
			t.Fatalf("Put(...): unexpected error: %v", err)
		}
	})
}

func TestNextTagID(t *testing.T) {
	t.Parallel()

	ts := newTestStorage(t)

	for i := 1; i < 4; i++ {
		id, err := upload.NextTag(ts.IndexStore())
		if err != nil {
			t.Fatal(err)
		}

		if id != uint64(i) {
			t.Fatalf("incorrect tag ID returned, exp: %d found %d", i, id)
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
