// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package soc

import (
	"bytes"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// Valid checks if the chunk is a valid single-owner chunk.
func Valid(ch swarm.Chunk) bool {
	s, err := FromChunk(ch)
	if err != nil {
		return false
	}

	// disperse replica validation
	if bytes.Equal(s.owner, swarm.ReplicasOwner) && !bytes.Equal(s.WrappedChunk().Address().Bytes()[1:32], s.id[1:32]) {
		return false
	}

	address, err := s.Address()
	if err != nil {
		return false
	}
	return ch.Address().Equal(address)
}
