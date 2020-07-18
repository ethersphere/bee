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

// TestContentChunkWithSpan verifies creation of content addressed chunk from
// byte data.
func TestContentChunk(t *testing.T) {
	bmtHashOfFoo := "2387e8e7d8a48c2a9339c97c1dc3461a9a7aa07e994c5cb8b38fd7c1b3e6ea48"
	address := swarm.MustParseHexAddress(bmtHashOfFoo)

	c, err := content.NewContentChunk([]byte("foo"))
	if err != nil {
		t.Fatal(err)
	}
	if !address.Equal(c.Address()) {
		t.Fatal("address mismatch")
	}
}

// TestContentChunkWithSpan verifies creation of content addressed chunk from
// payload data and span in integer form.
func TestContentChunkWithSpan(t *testing.T) {
	bmtHashOfFoo := "2387e8e7d8a48c2a9339c97c1dc3461a9a7aa07e994c5cb8b38fd7c1b3e6ea48"
	address := swarm.MustParseHexAddress(bmtHashOfFoo)

	data := []byte("foo")
	c, err := content.NewContentChunkWithSpan(data, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if !address.Equal(c.Address()) {
		t.Fatal("address mismatch")
	}
}

// TestContentChunkWithSpanBytes verifies creation of content addressed chunk from
// payload data and span in byte form.
func TestContentChunkWithSpanBytes(t *testing.T) {
	bmtHashOfFoo := "2387e8e7d8a48c2a9339c97c1dc3461a9a7aa07e994c5cb8b38fd7c1b3e6ea48"
	address := swarm.MustParseHexAddress(bmtHashOfFoo)

	data := []byte("foo")
	span := len(data)
	spanBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(spanBytes, uint64(span))
	c, err := content.NewContentChunkWithSpanBytes(data, spanBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !address.Equal(c.Address()) {
		t.Fatal("address mismatch")
	}
}
