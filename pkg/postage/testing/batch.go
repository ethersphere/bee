// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"bytes"
	crand "crypto/rand"
	"io"
	"math/big"
	"math/rand"
	"testing"

	"github.com/ethersphere/bee/pkg/postage"
)

const (
	defaultBucketDepth = 12
	defaultDepth       = 16
)

// BatchOption is an optional parameter for NewBatch
type BatchOption func(c *postage.Batch)

// MustNewID will generate a new random ID (32 byte slice). Panics on errors.
func MustNewID() []byte {
	id := make([]byte, 32)
	_, err := io.ReadFull(crand.Reader, id)
	if err != nil {
		panic(err)
	}
	return id
}

// MustNewAddress will generate a new random address (20 byte slice). Panics on
// errors.
func MustNewAddress() []byte {
	addr := make([]byte, 20)
	_, err := io.ReadFull(crand.Reader, addr)
	if err != nil {
		panic(err)
	}
	return addr
}

// NewBigInt will generate a new random big int (uint64 base value).
func NewBigInt() *big.Int {
	return (new(big.Int)).SetUint64(rand.Uint64()) // skipcq: GSC-G404
}

// MustNewBatch will create a new test batch. Fields that are not supplied will
// be filled with random data. Panics on errors.
func MustNewBatch(opts ...BatchOption) *postage.Batch {
	b := &postage.Batch{
		ID:          MustNewID(),
		Value:       NewBigInt(),
		Start:       rand.Uint64(), // skipcq: GSC-G404
		BucketDepth: defaultBucketDepth,
		Depth:       defaultDepth,
		Immutable:   true,
	}

	for _, opt := range opts {
		opt(b)
	}

	if b.Owner == nil {
		b.Owner = MustNewAddress()
	}

	return b
}

// WithOwner will set the batch owner on a randomized batch.
func WithOwner(owner []byte) BatchOption {
	return func(b *postage.Batch) {
		b.Owner = owner
	}
}

// CompareBatches is a testing helper that compares two batches and fails the
// test if all fields are not equal.
// Fails on first different value and prints the comparison.
func CompareBatches(t *testing.T, want, got *postage.Batch) {
	t.Helper()

	if !bytes.Equal(want.ID, got.ID) {
		t.Fatalf("batch ID: want %v, got %v", want.ID, got.ID)
	}
	if want.Value.Cmp(got.Value) != 0 {
		t.Fatalf("value: want %v, got %v", want.Value, got.Value)
	}
	if want.Start != got.Start {
		t.Fatalf("start: want %v, got %b", want.Start, got.Start)
	}
	if !bytes.Equal(want.Owner, got.Owner) {
		t.Fatalf("owner: want %v, got %v", want.Owner, got.Owner)
	}
	if want.Depth != got.Depth {
		t.Fatalf("depth: want %v, got %v", want.Depth, got.Depth)
	}
}
