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
	s := postage.NewStamp(nil, nil, nil)
	if err := s.UnmarshalBinary(buf); err != nil {
		t.Fatalf("unexpected error unmarshalling stamp: %v", err)
	}
	if !bytes.Equal(sExp.BatchID(), s.BatchID()) {
		t.Fatalf("id mismatch, expected %x, got %x", sExp.BatchID(), s.BatchID())
	}
	if !bytes.Equal(sExp.Index(), s.Index()) {
		t.Fatalf("index mismatch, expected %x, got %x", sExp.Index(), s.Index())
	}
	if !bytes.Equal(sExp.Sig(), s.Sig()) {
		t.Fatalf("sig mismatch, expected %x, got %x", sExp.Sig(), s.Sig())
	}

}

func newStamp(t *testing.T) *postage.Stamp {
	const idSize = 32
	const signatureSize = 65

	id := make([]byte, idSize)
	if _, err := io.ReadFull(crand.Reader, id); err != nil {
		panic(err)
	}

	index := make([]byte, idSize)
	if _, err := io.ReadFull(crand.Reader, index); err != nil {
		t.Fatal(err)
	}

	sig := make([]byte, signatureSize)
	if _, err := io.ReadFull(crand.Reader, sig); err != nil {
		t.Fatal(err)
	}
	return postage.NewStamp(id, sig, index)
}
