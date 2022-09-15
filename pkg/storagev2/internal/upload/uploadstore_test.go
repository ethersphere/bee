// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package upload_test

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"

	chunktest "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/storagev2/inmemchunkstore"
	inmem "github.com/ethersphere/bee/pkg/storagev2/inmemstore"
	"github.com/ethersphere/bee/pkg/storagev2/internal"
	"github.com/ethersphere/bee/pkg/storagev2/internal/upload"
	"github.com/ethersphere/bee/pkg/storagev2/storagetest"
	"github.com/ethersphere/bee/pkg/swarm"
	swarmtesting "github.com/ethersphere/bee/pkg/swarm/test"
)

var _ internal.Storage = (*testStorage)(nil)

// testStorage is an implementation of internal.Storage for test purposes.
type testStorage struct {
	ctx        context.Context
	indexStore storage.Store
	chunkStore storage.ChunkStore
}

func (t *testStorage) Ctx() context.Context           { return t.ctx }
func (t *testStorage) Storage() storage.Store         { return t.indexStore }
func (t *testStorage) ChunkStore() storage.ChunkStore { return t.chunkStore }

func TestTagIDAddressItem_MarshalAndUnmarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		test *storagetest.ItemMarshalAndUnmarshalTest
	}{{
		name: "zero values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:       &upload.TagIDAddressItem{},
			Factory:    func() storage.Item { return new(upload.TagIDAddressItem) },
			MarshalErr: upload.ErrTagIDAddressItemMarshalAddressIsZero,
		},
	}, {
		name: "zero address",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &upload.TagIDAddressItem{
				Address: swarm.ZeroAddress,
				TagID:   1,
			},
			Factory:    func() storage.Item { return new(upload.TagIDAddressItem) },
			MarshalErr: upload.ErrTagIDAddressItemMarshalAddressIsZero,
		},
	}, {
		name: "min values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &upload.TagIDAddressItem{
				Address: swarm.NewAddress(storagetest.MinAddressBytes[:]),
				TagID:   0,
			},
			Factory: func() storage.Item { return new(upload.TagIDAddressItem) },
		},
	}, {
		name: "max values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &upload.TagIDAddressItem{
				Address: swarm.NewAddress(storagetest.MaxAddressBytes[:]),
				TagID:   math.MaxUint64,
			},
			Factory: func() storage.Item { return new(upload.TagIDAddressItem) },
		},
	}, {
		name: "random values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &upload.TagIDAddressItem{
				Address: swarmtesting.RandomAddress(),
				TagID:   rand.Uint64(),
			},
			Factory: func() storage.Item { return new(upload.TagIDAddressItem) },
		},
	}, {
		name: "invalid size",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &storagetest.ItemStub{
				MarshalBuf:   []byte{0xFF},
				UnmarshalBuf: []byte{0xFF},
			},
			Factory:      func() storage.Item { return new(upload.TagIDAddressItem) },
			UnmarshalErr: upload.ErrTagIDAddressItemUnmarshalInvalidSize,
		},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			storagetest.TestItemMarshalAndUnmarshal(t, tc.test)
		})
	}
}

func TestPushItem_MarshalAndUnmarshal(t *testing.T) {
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
		name: "min values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &upload.PushItem{
				Timestamp: 0,
				Address:   swarm.NewAddress(storagetest.MinAddressBytes[:]),
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
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			storagetest.TestItemMarshalAndUnmarshal(t, tc.test)
		})
	}
}

func TestChunkPutter(t *testing.T) {
	t.Parallel()

	now := func() time.Time { return time.Unix(1234567890, 0) }
	upload.ReplaceTimeNow(now)
	t.Cleanup(func() { upload.ReplaceTimeNow(time.Now) })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ts := &testStorage{
		ctx:        ctx,
		indexStore: inmem.New(),
		chunkStore: inmemchunkstore.New(),
	}
	t.Cleanup(func() {
		if err := ts.Storage().Close(); err != nil {
			t.Errorf("Storage().Close(): unexpected error: %v", err)
		}
		if err := ts.ChunkStore().Close(); err != nil {
			t.Errorf("ChunkStore().Close(): unexpected error: %v", err)
		}
	})

	tagID := uint64(1)
	putter, err := upload.ChunkPutter(ts, tagID)
	if err != nil {
		t.Fatalf("upload.ChunkPutter(...): unexpected error: %v", err)
	}

	for _, chunk := range chunktest.GenerateTestRandomChunks(10) {
		t.Run(fmt.Sprintf("chunk %s", chunk.Address()), func(t *testing.T) {
			t.Run("put new chunk", func(t *testing.T) {
				exists, err := putter.Put(context.TODO(), chunk)
				if err != nil {
					t.Fatalf("Put(...): unexpected error: %v", err)
				}
				if exists {
					t.Fatal("Put(...): chunk should not exist")
				}
			})

			t.Run("put existing chunk", func(t *testing.T) {
				exists, err := putter.Put(context.TODO(), chunk)
				if err != nil {
					t.Fatalf("Put(...): unexpected error: %v", err)
				}
				if !exists {
					t.Fatal("Put(...): chunk should exist")
				}
			})

			t.Run("verify internal state", func(t *testing.T) {
				has, err := ts.Storage().Has(&upload.TagIDAddressItem{
					TagID:   tagID,
					Address: chunk.Address(),
				})
				if err != nil {
					t.Fatalf("Has(...): unexpected error: %v", err)
				}
				if !has {
					t.Fatal("Has(...): item not found")
				}

				has, err = ts.Storage().Has(&upload.PushItem{
					Timestamp: now().Unix(),
					Address:   chunk.Address(),
					TagID:     tagID,
				})
				if err != nil {
					t.Fatalf("Has(...): unexpected error: %v", err)
				}
				if !has {
					t.Fatalf("Has(...): item not found")
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
}
