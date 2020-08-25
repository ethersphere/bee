// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pipeline

import (
	"github.com/ethersphere/bee/pkg/storage"
)

type pipeWriteArgs struct {
	ref  []byte
	key  []byte
	span []byte
	data []byte //data includes the span too!
}

type Pipeline struct {
	head Interface
}

func (p *Pipeline) Write(b []byte) (int, error) {
	return p.head.Write(b)
}

func (p *Pipeline) Sum() ([]byte, error) {
	return p.head.Sum()
}

func NewPipeline(s storage.Storer) Interface {
	// DATA -> FEEDER -> BMT -> STORAGE -> TRIE
	tw := NewHashTrieWriter(4096, 128, 32, NewShortPipeline(s))
	lsw := NewStoreWriter(s, tw)
	b := NewBmtWriter(128, lsw)
	feeder := NewChunkFeederWriter(4096, b)

	return &Pipeline{head: feeder}
}

type pipelineFunc func(p *pipeWriteArgs) ChainWriter

// this is just a hashing pipeline. needed for level wrapping inside the hash trie writer
func NewShortPipeline(s storage.Storer) func(*pipeWriteArgs) ChainWriter {
	return func(p *pipeWriteArgs) ChainWriter {
		rsw := NewResultWriter(p)
		lsw := NewStoreWriter(s, rsw)
		bw := NewBmtWriter(128, lsw)

		return bw
	}
}

//func NewEncryptionPipeline() EndPipeWriter {
//tw := NewHashTrieWriter()
//lsw := NewStoreWriter()
//b := NewEncryptingBmtWriter(128, lsw) // needs to pass the key somehwoe...
//enc := NewEncryptionWriter(b)
//return
//}
