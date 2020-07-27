// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package content_test

import (
	"encoding/binary"
	"testing"

	"github.com/ethersphere/bee/pkg/content"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestChunkWithSpan verifies creation of content addressed chunk from
// byte data.
func TestChunk(t *testing.T) {
	bmtHashOfFoo := "2387e8e7d8a48c2a9339c97c1dc3461a9a7aa07e994c5cb8b38fd7c1b3e6ea48"
	address := swarm.MustParseHexAddress(bmtHashOfFoo)

	c, err := content.NewChunk([]byte("foo"))
	if err != nil {
		t.Fatal(err)
	}
	if !address.Equal(c.Address()) {
		t.Fatal("address mismatch")
	}
}

// TestChunkWithSpan verifies creation of content addressed chunk from
// payload data and span in integer form.
func TestChunkWithSpan(t *testing.T) {
	bmtHashOfFoo := "2387e8e7d8a48c2a9339c97c1dc3461a9a7aa07e994c5cb8b38fd7c1b3e6ea48"
	address := swarm.MustParseHexAddress(bmtHashOfFoo)

	data := []byte("foo")
	c, err := content.NewChunkWithSpan(data, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if !address.Equal(c.Address()) {
		t.Fatal("address mismatch")
	}
}

// TestChunkWithSpanBytes verifies creation of content addressed chunk from
// payload data and span in byte form.
func TestChunkWithSpanBytes(t *testing.T) {
	bmtHashOfFoo := "2387e8e7d8a48c2a9339c97c1dc3461a9a7aa07e994c5cb8b38fd7c1b3e6ea48"
	address := swarm.MustParseHexAddress(bmtHashOfFoo)

	data := []byte("foo")
	span := len(data)
	spanBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(spanBytes, uint64(span))
	c, err := content.NewChunkWithSpanBytes(data, spanBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !address.Equal(c.Address()) {
		t.Fatal("address mismatch")
	}
}
