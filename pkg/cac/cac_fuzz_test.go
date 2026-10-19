// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cac_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// FuzzValid fuzzes content-addressed-chunk validation, which runs on
// peer-supplied chunk data across every chunk-bearing protocol. The span
// prefix drives the BMT hashing, so the target ensures Valid never panics on
// truncated, empty, or oversized data regardless of the claimed address.
func FuzzValid(f *testing.F) {
	ch, err := cac.New([]byte("hello world"))
	if err != nil {
		f.Fatal(err)
	}
	f.Add(ch.Address().Bytes(), ch.Data())
	f.Add([]byte{}, []byte{})
	f.Add(make([]byte, swarm.HashSize), make([]byte, swarm.SpanSize))
	f.Add(make([]byte, swarm.HashSize), make([]byte, swarm.ChunkWithSpanSize))
	f.Add(make([]byte, swarm.HashSize), make([]byte, swarm.ChunkWithSpanSize+1))

	f.Fuzz(func(t *testing.T, addr, data []byte) {
		ch := swarm.NewChunk(swarm.NewAddress(addr), data)
		_ = cac.Valid(ch) // must not panic
	})
}

// FuzzNewWithDataSpan fuzzes the span+data constructor used when unwrapping the
// CAC embedded in a single-owner chunk. It asserts the length arithmetic never
// panics and that any chunk it produces is self-consistent under Valid.
func FuzzNewWithDataSpan(f *testing.F) {
	f.Add([]byte("span-header-plus-some-payload-data"))
	f.Add([]byte{})
	f.Add(make([]byte, swarm.SpanSize-1))
	f.Add(make([]byte, swarm.SpanSize))
	f.Add(make([]byte, swarm.ChunkWithSpanSize))
	f.Add(make([]byte, swarm.ChunkWithSpanSize+1))

	f.Fuzz(func(t *testing.T, data []byte) {
		ch, err := cac.NewWithDataSpan(data)
		if err != nil {
			return
		}
		if !cac.Valid(ch) {
			t.Fatal("NewWithDataSpan produced a chunk that fails Valid")
		}
	})
}
