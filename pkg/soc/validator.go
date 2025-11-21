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

	// if the address does not match the chunk address, check if it is a disperse replica
	if !ch.Address().Equal(address) {
		c := ch.Address().Bytes()
		a := address.Bytes()
		// For disperse replicas it is allowed to have the first 4 bits of the first
		// byte to be different, and the last 4 bits must be equal.
		// Another case is when only the fifth bit from the left is flipped.
		return ((c[0]&0x0f == a[0]&0x0f) || (c[0]^a[0] == 1<<3)) && bytes.Equal(c[1:], a[1:])
	}

	return true
}
