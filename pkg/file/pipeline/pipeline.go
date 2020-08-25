// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pipeline

import (
	"context"
	"fmt"
	"io"

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
func NewPipeline(ctx context.Context, s storage.Storer, mode storage.ModePut) Interface {
	tw := newHashTrieWriter(swarm.ChunkSize, swarm.Branches, swarm.HashSize, newShortPipelineFunc(ctx, s, mode))
	lsw := newStoreWriter(ctx, s, mode, tw)
	b := newBmtWriter(128, lsw)
	return newChunkFeederWriter(swarm.ChunkSize, b)
}

type pipelineFunc func() chainWriter

// newShortPipelineFunc returns a constructor function for an ephemeral hashing pipeline
// needed by the hashTrieWriter.
func newShortPipelineFunc(ctx context.Context, s storage.Storer, mode storage.ModePut) func() chainWriter {
	return func() chainWriter {
		lsw := newStoreWriter(ctx, s, mode, nil)
		return newBmtWriter(128, lsw)
	}
}

// FeedPipeline feeds the pipeline with the given reader until EOF is reached.
// It returns the cryptographic root hash of the content.
func FeedPipeline(ctx context.Context, pipeline Interface, r io.Reader, dataLength int64) (addr swarm.Address, err error) {
	var total int64
	data := make([]byte, swarm.ChunkSize)
	var eof bool
	for !eof {
		c, err := r.Read(data)
		total += int64(c)
		if err != nil {
			if err == io.EOF {
				if total < dataLength {
					return swarm.ZeroAddress, fmt.Errorf("pipline short write: read %d out of %d bytes", total+int64(c), dataLength)
				}
				eof = true
				if c > 0 {
					cc, err := pipeline.Write(data[:c])
					if err != nil {
						return swarm.ZeroAddress, err
					}
					if cc < c {
						return swarm.ZeroAddress, fmt.Errorf("pipeline short write: %d mismatches %d", cc, c)
					}
				}
				continue
			} else {
				return swarm.ZeroAddress, err
			}
		}
		cc, err := pipeline.Write(data[:c])
		if err != nil {
			return swarm.ZeroAddress, err
		}
		if cc < c {
			return swarm.ZeroAddress, fmt.Errorf("pipeline short write: %d mismatches %d", cc, c)
		}
		select {
		case <-ctx.Done():
			return swarm.ZeroAddress, ctx.Err()
		default:
		}
	}
	select {
	case <-ctx.Done():
		return swarm.ZeroAddress, ctx.Err()
	default:
	}

	sum, err := pipeline.Sum()
	if err != nil {
		return swarm.ZeroAddress, err
	}

	newAddress := swarm.NewAddress(sum)
	return newAddress, nil
}
