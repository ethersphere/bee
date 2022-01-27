// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package soc_test

import (
	"strings"
	"testing"

	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/swarm"
)

// TestValid verifies that the validator can detect
// valid soc chunks.
func TestValid(t *testing.T) {
	socAddress := swarm.MustParseHexAddress("9d453ebb73b2fedaaf44ceddcf7a0aa37f3e3d6453fea5841c31f0ea6d61dc85")

	// signed soc chunk of:
	// id: 0
	// wrapped chunk of: `foo`
	// owner: 0x8d3766440f0d7b949a5e32995d09619a7f86e632
	sch := swarm.NewChunk(socAddress, []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 90, 205, 56, 79, 235, 193, 51, 183, 178, 69, 229, 221, 198, 45, 130, 210, 205, 237, 145, 130, 210, 113, 97, 38, 205, 136, 68, 80, 154, 246, 90, 5, 61, 235, 65, 130, 8, 2, 127, 84, 142, 62, 136, 52, 58, 246, 248, 74, 135, 114, 251, 60, 235, 192, 161, 131, 58, 14, 167, 236, 12, 19, 72, 49, 27, 3, 0, 0, 0, 0, 0, 0, 0, 102, 111, 111})

	// check valid chunk
	if !soc.Valid(sch) {
		t.Fatal("valid chunk evaluates to invalid")
	}
}

// TestInvalid verifies that the validator can detect chunks
// with invalid data and invalid address.
func TestInvalid(t *testing.T) {
	socAddress := swarm.MustParseHexAddress("9d453ebb73b2fedaaf44ceddcf7a0aa37f3e3d6453fea5841c31f0ea6d61dc85")

	// signed soc chunk of:
	// id: 0
	// wrapped chunk of: `foo`
	// owner: 0x8d3766440f0d7b949a5e32995d09619a7f86e632
	sch := swarm.NewChunk(socAddress, []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 90, 205, 56, 79, 235, 193, 51, 183, 178, 69, 229, 221, 198, 45, 130, 210, 205, 237, 145, 130, 210, 113, 97, 38, 205, 136, 68, 80, 154, 246, 90, 5, 61, 235, 65, 130, 8, 2, 127, 84, 142, 62, 136, 52, 58, 246, 248, 74, 135, 114, 251, 60, 235, 192, 161, 131, 58, 14, 167, 236, 12, 19, 72, 49, 27, 3, 0, 0, 0, 0, 0, 0, 0, 102, 111, 111})

	for _, c := range []struct {
		name  string
		chunk func() swarm.Chunk
	}{
		{
			name: "wrong soc address",
			chunk: func() swarm.Chunk {
				wrongAddressBytes := sch.Address().Bytes()
				wrongAddressBytes[0] = 255 - wrongAddressBytes[0]
				wrongAddress := swarm.NewAddress(wrongAddressBytes)
				return swarm.NewChunk(wrongAddress, sch.Data())
			},
		},
		{
			name: "invalid data",
			chunk: func() swarm.Chunk {
				data := make([]byte, len(sch.Data()))
				copy(data, sch.Data())
				cursor := swarm.HashSize + swarm.SignatureSize
				chunkData := data[cursor:]
				chunkData[0] = 0x01
				return swarm.NewChunk(socAddress, data)
			},
		},
		{
			name: "invalid id",
			chunk: func() swarm.Chunk {
				data := make([]byte, len(sch.Data()))
				copy(data, sch.Data())
				id := data[:swarm.HashSize]
				id[0] = 0x01
				return swarm.NewChunk(socAddress, data)
			},
		},
		{
			name: "invalid signature",
			chunk: func() swarm.Chunk {
				data := make([]byte, len(sch.Data()))
				copy(data, sch.Data())
				// modify signature
				cursor := swarm.HashSize + swarm.SignatureSize
				sig := data[swarm.HashSize:cursor]
				sig[0] = 0x01
				return swarm.NewChunk(socAddress, data)
			},
		},
		{
			name: "nil data",
			chunk: func() swarm.Chunk {
				return swarm.NewChunk(socAddress, nil)
			},
		},
		{
			name: "small data",
			chunk: func() swarm.Chunk {
				return swarm.NewChunk(socAddress, []byte("small"))
			},
		},
		{
			name: "large data",
			chunk: func() swarm.Chunk {
				return swarm.NewChunk(socAddress, []byte(strings.Repeat("a", swarm.ChunkSize+swarm.SpanSize+1)))
			},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if soc.Valid(c.chunk()) {
				t.Fatal("chunk with invalid data evaluates to valid")
			}
		})
	}
}
