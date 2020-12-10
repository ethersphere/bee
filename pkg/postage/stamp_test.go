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
