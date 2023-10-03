// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	"github.com/ethersphere/bee/pkg/bmtpool"
	"github.com/ethersphere/bee/pkg/cac"
	storer "github.com/ethersphere/bee/pkg/storer"
	"github.com/ethersphere/bee/pkg/swarm"
)

func sampleChunk(items []storer.SampleItem) (swarm.Chunk, error) {
	contentSize := len(items) * 2 * swarm.HashSize

	pos := 0
	content := make([]byte, contentSize)
	for _, s := range items {
		copy(content[pos:], s.ChunkAddress.Bytes())
		pos += swarm.HashSize
		copy(content[pos:], s.TransformedAddress.Bytes())
		pos += swarm.HashSize
	}

	return cac.New(content)
}

func sampleHash(items []storer.SampleItem) (swarm.Address, error) {
	hasher := bmtpool.Get()
	defer bmtpool.Put(hasher)

	for _, s := range items {
		_, err := hasher.Write(s.TransformedAddress.Bytes())
		if err != nil {
			return swarm.ZeroAddress, err
		}
	}
	hash := hasher.Sum(nil)

	return swarm.NewAddress(hash), nil

	// PH4_Logic:
	// ch, err := sampleChunk(items)
	// if err != nil {
	// 	return swarm.ZeroAddress, err
	// }
	// return ch.Address(), nil
}
