// Copyright 2019 The Swarm Authors
// This file is part of the Swarm library.
//
// The Swarm library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Swarm library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Swarm library. If not, see <http://www.gnu.org/licenses/>.

package testing

import (
	"math/rand"
	"time"

	"github.com/ethersphere/bee/pkg/swarm"
)

func init() {
	// needed for GenerateTestRandomChunk
	rand.Seed(time.Now().UnixNano())
}

// GenerateTestRandomChunk generates a Chunk that is not
// valid, but it contains a random key and a random value.
// This function is faster then storage.GenerateRandomChunk
// which generates a valid chunk.
// Some tests in do not need valid chunks, just
// random data, and their execution time can be decreased
// using this function.
func GenerateTestRandomChunk() swarm.Chunk {
	data := make([]byte, swarm.ChunkSize)
	_, _ = rand.Read(data)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	return swarm.NewChunk(swarm.NewAddress(key), data)
}

// GenerateTestRandomChunks generates a slice of random
// Chunks by using GenerateTestRandomChunk function.
func GenerateTestRandomChunks(count int) []swarm.Chunk {
	chunks := make([]swarm.Chunk, count)
	for i := 0; i < count; i++ {
		chunks[i] = GenerateTestRandomChunk()
	}
	return chunks
}
