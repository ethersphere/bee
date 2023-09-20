// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage_test

import (
	crand "crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sync"
	"testing"

	"github.com/ethersphere/bee/pkg/postage"
	storage "github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/storagetest"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// TestStampIssuerMarshalling tests the idempotence  of binary marshal/unmarshal.
func TestStampIssuerMarshalling(t *testing.T) {
	want := newTestStampIssuer(t, 1000)
	buf, err := want.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	have := &postage.StampIssuer{}
	err = have.UnmarshalBinary(buf)
	if err != nil {
		t.Fatal(err)
	}

	opts := []cmp.Option{
		cmp.AllowUnexported(postage.StampIssuer{}, big.Int{}),
		cmpopts.IgnoreInterfaces(struct{ storage.Store }{}),
		cmpopts.IgnoreTypes(sync.Mutex{}, sync.RWMutex{}),
	}
	if !cmp.Equal(want, have, opts...) {
		t.Errorf("Marshal/Unmarshal mismatch (-want +have):\n%s", cmp.Diff(want, have))
	}
}

func newTestStampIssuer(t *testing.T, block uint64) *postage.StampIssuer {
	t.Helper()
	return newTestStampIssuerMutability(t, block, true)
}

func newTestStampIssuerMutability(t *testing.T, block uint64, immutable bool) *postage.StampIssuer {
	t.Helper()
	id := make([]byte, 32)
	_, err := io.ReadFull(crand.Reader, id)
	if err != nil {
		t.Fatal(err)
	}
	return postage.NewStampIssuer(
		"label",
		"keyID",
		id,
		big.NewInt(3),
		16,
		8,
		block,
		immutable,
	)
}

func TestStampItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		test *storagetest.ItemMarshalAndUnmarshalTest
	}{{
		name: "zero batchID",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:       postage.NewStampItem(),
			Factory:    func() storage.Item { return postage.NewStampItem() },
			MarshalErr: postage.ErrStampItemMarshalBatchIDInvalid,
			CmpOpts:    []cmp.Option{cmp.AllowUnexported(postage.StampItem{})},
		},
	}, {
		name: "zero chunkAddress",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item:       postage.NewStampItem().WithBatchID([]byte{swarm.HashSize - 1: 9}),
			Factory:    func() storage.Item { return postage.NewStampItem() },
			MarshalErr: postage.ErrStampItemMarshalChunkAddressInvalid,
			CmpOpts:    []cmp.Option{cmp.AllowUnexported(postage.StampItem{})},
		},
	}, {
		name: "valid values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: postage.NewStampItem().
				WithBatchID([]byte{swarm.HashSize - 1: 9}).
				WithChunkAddress(swarm.RandAddress(t)).
				WithBatchIndex([]byte{swarm.StampIndexSize - 1: 9}).
				WithBatchTimestamp([]byte{swarm.StampTimestampSize - 1: 9}),
			Factory: func() storage.Item { return postage.NewStampItem() },
			CmpOpts: []cmp.Option{cmp.AllowUnexported(postage.StampItem{})},
		},
	}, {
		name: "max values",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: postage.NewStampItem().
				WithBatchID(storagetest.MaxAddressBytes[:]).
				WithChunkAddress(swarm.NewAddress(storagetest.MaxAddressBytes[:])).
				WithBatchIndex(storagetest.MaxStampIndexBytes[:]).
				WithBatchTimestamp(storagetest.MaxBatchTimestampBytes[:]),
			Factory: func() storage.Item { return postage.NewStampItem() },
			CmpOpts: []cmp.Option{cmp.AllowUnexported(postage.StampItem{})},
		},
	}, {
		name: "invalid size",
		test: &storagetest.ItemMarshalAndUnmarshalTest{
			Item: &storagetest.ItemStub{
				MarshalBuf:   []byte{0xFF},
				UnmarshalBuf: []byte{0xFF},
			},
			Factory:      func() storage.Item { return postage.NewStampItem() },
			UnmarshalErr: postage.ErrStampItemUnmarshalInvalidSize,
			CmpOpts:      []cmp.Option{cmp.AllowUnexported(postage.StampItem{})},
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

func Test_StampIssuer_inc(t *testing.T) {
	t.Parallel()

	addr := swarm.NewAddress([]byte{1, 2, 3, 4})

	t.Run("mutable", func(t *testing.T) {
		t.Parallel()

		sti := postage.NewStampIssuer("label", "keyID", make([]byte, 32), big.NewInt(3), 16, 8, 0, false)
		count := sti.BucketUpperBound()

		// Increment to upper bound (fill bucket to max cap)
		for i := uint32(0); i < count; i++ {
			_, _, err := sti.Increment(addr)
			if err != nil {
				t.Fatal(err)
			}
		}

		// Incrementing stamp issuer above upper bound should return index starting from 0
		for i := uint32(0); i < count; i++ {
			idxb, _, err := sti.Increment(addr)
			if err != nil {
				t.Fatal(err)
			}

			if _, idx := bytesToIndex(idxb); idx != i {
				t.Fatalf("bucket should be full %v", idx)
			}
		}
	})

	t.Run("immutable", func(t *testing.T) {
		t.Parallel()

		sti := postage.NewStampIssuer("label", "keyID", make([]byte, 32), big.NewInt(3), 16, 8, 0, true)
		count := sti.BucketUpperBound()

		// Increment to upper bound (fill bucket to max cap)
		for i := uint32(0); i < count; i++ {
			_, _, err := sti.Increment(addr)
			if err != nil {
				t.Fatal(err)
			}
		}

		// Incrementing stamp issuer above upper bound should return error
		for i := uint32(0); i < count; i++ {
			_, _, err := sti.Increment(addr)
			if !errors.Is(err, postage.ErrBucketFull) {
				t.Fatal("bucket should be full")
			}
		}
	})
}

func bytesToIndex(buf []byte) (bucket, index uint32) {
	index64 := binary.BigEndian.Uint64(buf)
	bucket = uint32(index64 >> 32)
	index = uint32(index64)
	return bucket, index
}
