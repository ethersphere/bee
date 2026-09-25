// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage_test

import (
	"bytes"
	"errors"
	"math/big"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/postage"
	postagetesting "github.com/ethersphere/bee/v2/pkg/postage/testing"
)

// TestBatchMarshalling tests the idempotence  of binary marshal/unmarshal for a
// Batch.
func TestBatchMarshalling(t *testing.T) {
	t.Parallel()

	a := postagetesting.MustNewBatch()
	buf, err := a.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if len(buf) != 95 {
		t.Fatalf("invalid length for serialised batch. expected 95, got %d", len(buf))
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
	if a.BucketDepth != b.BucketDepth {
		t.Fatalf("bucket depth mismatch, expected %d, got %d", a.BucketDepth, b.BucketDepth)
	}
	if a.Immutable != b.Immutable {
		t.Fatalf("depth mismatch, expected %v, got %v", a.Immutable, b.Immutable)
	}
}

// TestBatchUnmarshalBufferLength tests that a truncated buffer is rejected
// rather than panicking, and that the legacy 96 byte serialisation is still
// accepted.
func TestBatchUnmarshalBufferLength(t *testing.T) {
	t.Parallel()

	valid, err := postagetesting.MustNewBatch().MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	for _, buf := range [][]byte{nil, {}, []byte("0"), valid[:94]} {
		b := &postage.Batch{}
		if err := b.UnmarshalBinary(buf); !errors.Is(err, postage.ErrBatchInvalid) {
			t.Fatalf("expected %v for buffer of length %d, got %v", postage.ErrBatchInvalid, len(buf), err)
		}
	}

	// legacy encoding carried a trailing storage radius byte.
	legacy := append(append([]byte{}, valid...), 0)
	b := &postage.Batch{}
	if err := b.UnmarshalBinary(legacy); err != nil {
		t.Fatalf("unexpected error unmarshalling legacy batch: %v", err)
	}
}

// TestBatchMarshalOversizedFields tests that oversized fields are rejected
// rather than corrupting neighbouring fields or panicking.
func TestBatchMarshalOversizedFields(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		mod  func(*postage.Batch)
	}{
		{"nil value", func(b *postage.Batch) { b.Value = nil }},
		{"oversized value", func(b *postage.Batch) { b.Value = new(big.Int).Lsh(big.NewInt(1), 8*64) }},
		{"oversized id", func(b *postage.Batch) { b.ID = make([]byte, 33) }},
		{"oversized owner", func(b *postage.Batch) { b.Owner = make([]byte, 21) }},
	} {
		b := postagetesting.MustNewBatch()
		tc.mod(b)
		if _, err := b.MarshalBinary(); !errors.Is(err, postage.ErrBatchInvalid) {
			t.Fatalf("%s: expected %v, got %v", tc.name, postage.ErrBatchInvalid, err)
		}
	}
}
