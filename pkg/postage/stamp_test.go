// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage_test

import (
	"bytes"
	crand "crypto/rand"
	"io"
	"math/big"
	"math/rand"
	"testing"

	"github.com/ethersphere/bee/pkg/postage"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
)

// TestStampMarshalling tests the idempotence  of binary marshal/unmarshals for Stamps.
func TestStampMarshalling(t *testing.T) {

	sExp := postagetesting.NewStamp()
	buf, _ := sExp.MarshalBinary()
	if len(buf) != postage.StampSize {
		t.Fatalf("invalid length for serialised stamp. expected %d, got  %d", postage.StampSize, len(buf))
	}
	s := postage.NewStamp(nil, nil)
	if err := s.UnmarshalBinary(buf); err != nil {
		t.Fatalf("unexpected error unmarshalling stamp: %v", err)
	}
	if !bytes.Equal(sExp.BatchID(), s.BatchID()) {
		t.Fatalf("id mismatch, expected %x, got %x", sExp.BatchID(), s.BatchID())
	}
	if !bytes.Equal(sExp.Sig(), s.Sig()) {
		t.Fatalf("sig mismatch, expected %x, got %x", sExp.Sig(), s.Sig())
	}

}

// TestBatchMarshalling tests the idempotence  of binary marshal/unmarshal for a Batch.
func TestBatchMarshalling(t *testing.T) {
	a := newTestBatch(t, nil)
	buf, err := a.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if len(buf) != 93 {
		t.Fatalf("invalid length for serialised batch. expected 93, got %d", len(buf))
	}
	b := &postage.Batch{}
	if err := b.UnmarshalBinary(buf); err != nil {
		t.Fatalf("unexpected error unmarshalling batch: %v", err)
	}
	if !bytes.Equal(b.ID, a.ID) {
		t.Fatalf("id mismatch, expected %x, got %x", a.ID, b.ID)
	}
	if !bytes.Equal(b.Owner, a.Owner) {
		t.Fatalf("owner mismatch, expected %x, got %x", a.Owner, b.Owner)
	}
	if a.Value.Uint64() != b.Value.Uint64() {
		t.Fatalf("value mismatch, expected %d, got %d", a.Value.Uint64(), b.Value.Uint64())
	}
	if a.Start != b.Start {
		t.Fatalf("start mismatch, expected %d, got %d", a.Start, b.Start)
	}
	if a.Depth != b.Depth {
		t.Fatalf("depth mismatch, expected %d, got %d", a.Depth, b.Depth)
	}

}

func newTestBatch(t *testing.T, owner []byte) *postage.Batch {
	t.Helper()
	id := make([]byte, 32)
	_, err := io.ReadFull(crand.Reader, id)
	if err != nil {
		t.Fatal(err)
	}
	value64 := rand.Uint64()
	start64 := rand.Uint64()
	if owner == nil {
		owner = make([]byte, 20)
		_, err = io.ReadFull(crand.Reader, owner)
		if err != nil {
			t.Fatal(err)
		}
	}
	depth := uint8(16)
	return &postage.Batch{
		ID:    id,
		Value: big.NewInt(0).SetUint64(value64),
		Start: start64,
		Owner: owner,
		Depth: depth,
	}
}
