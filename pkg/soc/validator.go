// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package soc

import (
	"bytes"
	"encoding/hex"
	"fmt"

	"github.com/ethersphere/bee/pkg/swarm"
)

// Valid checks if the chunk is a valid single-owner chunk.
func Valid(ch swarm.Chunk) bool {
	s, err := FromChunk(ch)
	if err != nil {
		fmt.Printf("soc FromChunk: %v\n", err)
		return false
	}

	// disperse replica validation
	if bytes.Equal(s.owner, swarm.ReplicasOwner) && !bytes.Equal(s.WrappedChunk().Address().Bytes()[1:32], s.id[1:32]) {
		fmt.Printf("soc replica owner validation\n")
		return false
	}

	address, err := s.Address()
	if err != nil {
		fmt.Printf("soc get address\n")
		return false
	}

	if !ch.Address().Equal(address) {
		fmt.Printf("soc address not equal to chunk address. chunkaddress= %s, soc owner=%s, soc id=%s", ch.Address().String(), hex.EncodeToString(s.owner), hex.EncodeToString(s.id))
	}
	return ch.Address().Equal(address)
}
