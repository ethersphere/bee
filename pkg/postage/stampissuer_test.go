// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage_test

import (
	crand "crypto/rand"
	"errors"
	"io"
	"math/big"
	"reflect"
	"testing"

	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestStampIssuerMarshalling tests the idempotence  of binary marshal/unmarshal.
func TestStampIssuerMarshalling(t *testing.T) {
	st := newTestStampIssuer(t, 1000)
	buf, err := st.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	st0 := &postage.StampIssuer{}
	err = st0.UnmarshalBinary(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(st, st0) {
		t.Fatalf("unmarshal(marshal(StampIssuer)) != StampIssuer \n%v\n%v", st, st0)
	}
}

func newTestStampIssuer(t *testing.T, block uint64) *postage.StampIssuer {
	t.Helper()
	id := make([]byte, 32)
	_, err := io.ReadFull(crand.Reader, id)
	if err != nil {
		t.Fatal(err)
	}
	return postage.NewStampIssuer("label", "keyID", id, big.NewInt(3), 16, 8, block, true)
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
			_, err := sti.Inc(addr)
			if err != nil {
				t.Fatal(err)
			}
		}

		// Incrementing stamp issuer above upper bound should return index starting from 0
		for i := uint32(0); i < count; i++ {
			idxb, err := sti.Inc(addr)
			if err != nil {
				t.Fatal(err)
			}

			if _, idx := postage.BytesToIndex(idxb); idx != i {
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
			_, err := sti.Inc(addr)
			if err != nil {
				t.Fatal(err)
			}
		}

		// Incrementing stamp issuer above upper bound should return error
		for i := uint32(0); i < count; i++ {
			_, err := sti.Inc(addr)
			if !errors.Is(err, postage.ErrBucketFull) {
				t.Fatal("bucket should be full")
			}
		}
	})
}
