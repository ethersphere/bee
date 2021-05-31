// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage_test

import (
	"bytes"
	crand "crypto/rand"
	"io"
	"testing"

	"github.com/ethersphere/bee/pkg/postage"
)

// TestStampMarshalling tests the idempotence  of binary marshal/unmarshals for Stamps.
func TestStampMarshalling(t *testing.T) {
	sExp := newStamp(t)
	buf, _ := sExp.MarshalBinary()
	if len(buf) != postage.StampSize {
		t.Fatalf("invalid length for serialised stamp. expected %d, got  %d", postage.StampSize, len(buf))
	}
	s := postage.NewStamp(nil, nil, nil, nil)
	if err := s.UnmarshalBinary(buf); err != nil {
		t.Fatalf("unexpected error unmarshalling stamp: %v", err)
	}
	if !bytes.Equal(sExp.BatchID(), s.BatchID()) {
		t.Fatalf("id mismatch, expected %x, got %x", sExp.BatchID(), s.BatchID())
	}
	if !bytes.Equal(sExp.Index(), s.Index()) {
		t.Fatalf("index mismatch, expected %x, got %x", sExp.Index(), s.Index())
	}
	if !bytes.Equal(sExp.Timestamp(), s.Timestamp()) {
		t.Fatalf("timestamp mismatch, expected %x, got %x", sExp.Index(), s.Index())
	}
	if !bytes.Equal(sExp.Sig(), s.Sig()) {
		t.Fatalf("sig mismatch, expected %x, got %x", sExp.Sig(), s.Sig())
	}
}

// TestStampIndexMarshalling tests the idempotence of stamp index serialisation.
func TestStampIndexMarshalling(t *testing.T) {
	var (
		expBucket uint32 = 11789
		expIndex  uint32 = 199999
	)
	index := postage.IndexToBytes(expBucket, expIndex)
	bucket, idx := postage.BytesToIndex(index)
	if bucket != expBucket {
		t.Fatalf("bucket mismatch. want %d, got %d", expBucket, bucket)
	}
	if idx != expIndex {
		t.Fatalf("index mismatch. want %d, got %d", expIndex, idx)
	}
}

func newStamp(t *testing.T) *postage.Stamp {
	const idSize = 32
	const indexSize = 8
	const signatureSize = 65

	id := make([]byte, idSize)
	if _, err := io.ReadFull(crand.Reader, id); err != nil {
		panic(err)
	}

	index := make([]byte, indexSize)
	if _, err := io.ReadFull(crand.Reader, index); err != nil {
		t.Fatal(err)
	}

	sig := make([]byte, signatureSize)
	if _, err := io.ReadFull(crand.Reader, sig); err != nil {
		t.Fatal(err)
	}
	return postage.NewStamp(id, index, postage.Timestamp(), sig)
}
