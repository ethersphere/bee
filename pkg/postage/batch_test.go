// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage_test

import (
	"bytes"
	"testing"

	"github.com/ethersphere/bee/pkg/postage"
	postagetesting "github.com/ethersphere/bee/pkg/postage/testing"
)

// TestBatchMarshalling tests the idempotence  of binary marshal/unmarshal for a
// Batch.
func TestBatchMarshalling(t *testing.T) {
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
