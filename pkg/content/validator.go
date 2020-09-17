// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package content

import (
	"github.com/ethersphere/bee/pkg/swarm"
)

var _ swarm.Validator = (*Validator)(nil)

// Validator validates that the address of a given chunk
// is the content address of its contents.
type Validator struct {
}

// NewValidator constructs a new Validator
func NewValidator() swarm.Validator {
	return &Validator{}
}

// Validate performs the validation check.
func (v *Validator) Validate(ch swarm.Chunk) (valid bool, cType swarm.ChunkType) {
	chunkData := ch.Data()
	rch, err := contentChunkFromBytes(chunkData)
	if err != nil {
		return false, swarm.InvalidChunk
	}

	address := ch.Address()
	yes := address.Equal(rch.Address())
	if yes {
		return true, swarm.ContentChunk
	}
	return false, swarm.InvalidChunk
}
