// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing

import (
	"encoding/binary"
	"math/rand"

	"github.com/ethersphere/bee/pkg/swarm"
)

// GenerateTestRandomFileChunk generates one single chunk with arbitrary content and address
func GenerateTestRandomFileChunk(address swarm.Address, spanLength, dataSize int) swarm.Chunk {
	data := make([]byte, dataSize+8)
	binary.LittleEndian.PutUint64(data, uint64(spanLength))
	_, _ = rand.Read(data[8:])
	key := make([]byte, swarm.SectionSize)
	if address.IsZero() {
		_, _ = rand.Read(key)
	} else {
		copy(key, address.Bytes())
	}
	return swarm.NewChunk(swarm.NewAddress(key), data)
}
