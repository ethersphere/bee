// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cac_test

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math/rand"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

func TestNew(t *testing.T) {
	t.Parallel()

	data := []byte("greaterthanspan")
	bmtHashOfData := "27913f1bdb6e8e52cbd5a5fd4ab577c857287edf6969b41efe926b51de0f4f23"
	address := swarm.MustParseHexAddress(bmtHashOfData)
	dataWithSpan := dataWithSpan(data)

	c, err := cac.New(data)
	if err != nil {
		t.Fatal(err)
	}

	if !c.Address().Equal(address) {
		t.Fatalf("address mismatch. got %s want %s", c.Address().String(), address.String())
	}

	if !bytes.Equal(c.Data(), dataWithSpan) {
		t.Fatalf("chunk data mismatch. got %x want %x", c.Data(), dataWithSpan)
	}
}

func TestNewWithDataSpan(t *testing.T) {
	t.Parallel()

	data := []byte("greaterthanspan")
	bmtHashOfData := "95022e6af5c6d6a564ee55a67f8455a3e18c511b5697c932d9e44f07f2fb8c53"
	address := swarm.MustParseHexAddress(bmtHashOfData)

	c, err := cac.NewWithDataSpan(data)
	if err != nil {
		t.Fatal(err)
	}

	if !c.Address().Equal(address) {
		t.Fatalf("address mismatch. got %s want %s", c.Address().String(), address.String())
	}

	if !bytes.Equal(c.Data(), data) {
		t.Fatalf("chunk data mismatch. got %x want %x", c.Data(), data)
	}
}

func TestChunkInvariantsNew(t *testing.T) {
	t.Parallel()

	for _, cc := range []struct {
		name    string
		data    []byte
		wantErr error
	}{
		{
			name:    "nil",
			data:    nil,
			wantErr: nil,
		},
		{
			name:    "zero data",
			data:    []byte{},
			wantErr: nil,
		},
		{
			name:    "too large data chunk",
			data:    testutil.RandBytes(t, swarm.ChunkSize+1),
			wantErr: cac.ErrChunkDataLarge,
		},
		{
			name:    "ok",
			data:    testutil.RandBytes(t, rand.Intn(swarm.ChunkSize)+1),
			wantErr: nil,
		},
	} {
		t.Run(cc.name, func(t *testing.T) {
			t.Parallel()

			_, err := cac.New(cc.data)
			if !errors.Is(err, cc.wantErr) {
				t.Fatalf("got %v want %v", err, cc.wantErr)
			}
		})
	}
}

func TestChunkInvariantsNewWithDataSpan(t *testing.T) {
	t.Parallel()

	for _, cc := range []struct {
		name    string
		data    []byte
		wantErr error
	}{
		{
			name:    "nil",
			data:    nil,
			wantErr: cac.ErrChunkSpanShort,
		},
		{
			name:    "zero data",
			data:    []byte{},
			wantErr: cac.ErrChunkSpanShort,
		},
		{
			name:    "small data",
			data:    make([]byte, swarm.SpanSize),
			wantErr: nil,
		},
		{
			name:    "too large data chunk",
			data:    testutil.RandBytes(t, swarm.ChunkSize+swarm.SpanSize+1),
			wantErr: cac.ErrChunkDataLarge,
		},
		{
			name:    "ok",
			data:    dataWithSpan(testutil.RandBytes(t, rand.Intn(swarm.ChunkSize)+1)),
			wantErr: nil,
		},
	} {
		t.Run(cc.name, func(t *testing.T) {
			t.Parallel()

			_, err := cac.NewWithDataSpan(cc.data)
			if !errors.Is(err, cc.wantErr) {
				t.Fatalf("got %v want %v", err, cc.wantErr)
			}
		})
	}
}

// TestValid checks whether a chunk is a valid content-addressed chunk
func TestValid(t *testing.T) {
	t.Parallel()

	data := "foo"
	bmtHashOfData := "2387e8e7d8a48c2a9339c97c1dc3461a9a7aa07e994c5cb8b38fd7c1b3e6ea48"
	address := swarm.MustParseHexAddress(bmtHashOfData)

	ch := swarm.NewChunk(address, dataWithSpan([]byte(data)))
	assertValidChunk(t, ch, true)

	ch, _ = cac.New([]byte("Digital Freedom Now"))
	assertValidChunk(t, ch, true)
}

// TestInvalid checks whether a chunk is not a valid content-addressed chunk
func TestInvalid(t *testing.T) {
	t.Parallel()

	// Generates a chunk with the given data. No validation is performed here,
	// the chunks are create as it is.
	chunker := func(addr string, dataBytes []byte) swarm.Chunk {
		addrBytes, _ := hex.DecodeString(addr)
		address := swarm.NewAddress(addrBytes)
		return swarm.NewChunk(address, dataBytes)
	}

	for _, tc := range []struct {
		name  string
		chunk swarm.Chunk
	}{
		{
			name: "wrong address",
			chunk: chunker(
				"0000e8e7d8a48c2a9339c97c1dc3461a9a7aa07e994c5cb8b38fd7c1b3e6ea48",
				dataWithSpan([]byte("foo")),
			),
		},
		{
			name:  "empty address",
			chunk: chunker("", dataWithSpan([]byte("foo"))),
		},
		{
			name:  "zero data",
			chunk: chunker("anything", []byte{}),
		},
		{
			name:  "nil data",
			chunk: chunker("anything", nil),
		},
		{
			name: "small data",
			chunk: chunker(
				"6251dbc53257832ae80d0e9f1cc41bd54d5b6c704c9c7349709c07fefef0aea6",
				[]byte("small"),
			),
		},
		{
			name: "small data (upper edge case)",
			chunk: chunker(
				"6251dbc53257832ae80d0e9f1cc41bd54d5b6c704c9c7349709c07fefef0aea6",
				dataWithSpan(nil),
			),
		},
		{
			name: "large data",
			chunk: chunker(
				"ffd70157e48063fc33c97a050f7f640233bf646cc98d9524c6b92bcf3ab56f83",
				testutil.RandBytes(t, swarm.ChunkSize+swarm.SpanSize+1),
			),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assertValidChunk(t, tc.chunk, false)
		})
	}
}

func assertValidChunk(t *testing.T, ch swarm.Chunk, expectValid bool) {
	t.Helper()

	isValid := cac.Valid(ch)
	if expectValid && !isValid {
		t.Fatalf("data '%x' should not have validated to hash '%s'", ch.Data(), ch.Address())
	} else if !expectValid && isValid {
		t.Fatalf("data '%x' should have validated to hash '%s'", ch.Data(), ch.Address())
	}
}

// dataWithSpan appends span to given input data
func dataWithSpan(inputData []byte) []byte {
	dataLength := len(inputData)
	dataBytes := make([]byte, swarm.SpanSize+dataLength)
	binary.LittleEndian.PutUint64(dataBytes, uint64(dataLength))
	copy(dataBytes[swarm.SpanSize:], inputData)
	return dataBytes
}
