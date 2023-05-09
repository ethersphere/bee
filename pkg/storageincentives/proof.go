// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
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
	ch, err := sampleChunk(items)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	return ch.Address(), nil
}
