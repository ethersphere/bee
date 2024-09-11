// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package soc

import "github.com/ethersphere/bee/v2/pkg/swarm"

// IdentityAddress returns the internally used address for the chunk
func IdentityAddress(chunk swarm.Chunk) (swarm.Address, error) {
	// check the chunk is single owner chunk or cac
	if sch, err := FromChunk(chunk); err == nil {
		socAddress, err := sch.Address()
		if err != nil {
			return swarm.ZeroAddress, err
		}
		h := swarm.NewHasher()
		_, err = h.Write(socAddress.Bytes())
		if err != nil {
			return swarm.ZeroAddress, err
		}
		_, err = h.Write(sch.WrappedChunk().Address().Bytes())
		if err != nil {
			return swarm.ZeroAddress, err
		}

		return swarm.NewAddress(h.Sum(nil)), nil
	}

	return chunk.Address(), nil
}
