// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package soc

import (
	"bytes"
	"errors"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var (
	errInvalidReplica  = errors.New("soc: replica id does not match wrapped chunk address")
	errAddressMismatch = errors.New("soc: chunk address does not match soc address")
)

// Valid checks if the chunk is a valid single-owner chunk.
func Valid(ch swarm.Chunk) bool {
	_, err := FromChunkValidate(ch)
	return err == nil
}

// FromChunkValidate parses the chunk as a single-owner chunk and verifies that
// the chunk address is the address of the parsed SOC. Unlike FromChunk, it
// rejects a correctly signed SOC delivered under any other address.
func FromChunkValidate(ch swarm.Chunk) (*SOC, error) {
	s, err := FromChunk(ch)
	if err != nil {
		return nil, err
	}

	// disperse replica validation
	if bytes.Equal(s.owner, swarm.ReplicasOwner) && !bytes.Equal(s.WrappedChunk().Address().Bytes()[1:32], s.id[1:32]) {
		return nil, errInvalidReplica
	}

	address, err := s.Address()
	if err != nil {
		return nil, err
	}
	if !ch.Address().Equal(address) {
		return nil, errAddressMismatch
	}
	return s, nil
}
