// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pipeline

import (
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

type pipeWriteArgs struct {
	ref  []byte
	span []byte
	data []byte //data includes the span too
}

// NewPipeline creates a standard pipeline that only hashes content with BMT to create
// a merkle-tree of hashes that represent the given arbitrary size byte stream. Partial
// writes are supported. The pipeline flow is: Data -> Feeder -> BMT -> Storage -> HashTrie.
func NewPipeline(s storage.Storer) Interface {
	tw := NewHashTrieWriter(swarm.ChunkSize, swarm.Branches, swarm.HashSize, NewShortPipelineFunc(s))
	lsw := NewStoreWriter(s, tw)
	b := NewBmtWriter(128, lsw)
	feeder := NewChunkFeederWriter(swarm.ChunkSize, b)

	return feeder
}

type pipelineFunc func(p *pipeWriteArgs) chainWriter

// NewShortPipelineFunc returns a constructor function for an ephemeral hashing pipeline
// needed by the hashTrieWriter.
func NewShortPipelineFunc(s storage.Storer) func(*pipeWriteArgs) chainWriter {
	return func(p *pipeWriteArgs) chainWriter {
		rsw := NewResultWriter(p)
		lsw := NewStoreWriter(s, rsw)
		bw := NewBmtWriter(128, lsw)

		return bw
	}
}
