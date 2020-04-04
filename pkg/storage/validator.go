// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.package storage

package storage

import (
	"github.com/ethersphere/bee/pkg/swarm"
)

// ChunkValidatorFunc validates Swarm chunk address and chunk data
type ChunkValidatorFunc func(chunk swarm.Chunk) bool

func ValidateContentChunk(ch swarm.Chunk) bool {

	if len(ch.Address().Bytes()) != swarm.DefaultAddressLength {
		return false
	}

	if len(ch.Data().Bytes()) > swarm.DefaultChunkSize {
		return false
	}
	return true
}
