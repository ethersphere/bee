// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hashtrie

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/file/pipeline"
	"github.com/ethersphere/bee/v2/pkg/file/redundancy"
	"github.com/ethersphere/bee/v2/pkg/replicas"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var (
	errInconsistentRefs = errors.New("inconsistent references")
	errTrieFull         = errors.New("trie full")
)

const maxLevel = 8

type hashTrieWriter struct {
	ctx                    context.Context // context for put function of dispersed replica chunks
	refSize                int
	cursors                []int  // level cursors, key is level. level 0 is data level holds how many chunks were processed. Intermediate higher levels will always have LOWER cursor values.
	buffer                 []byte // keeps intermediate level data
	full                   bool   // indicates whether the trie is full. currently we support (128^7)*4096 = 2305843009213693952 bytes
	pipelineFn             pipeline.PipelineFunc
	rParams                redundancy.RedundancyParams
	parityChunkFn          redundancy.ParityChunkCallback
	chunkCounters          []uint8        // counts the chunk references in intermediate chunks. key is the chunk level.
	effectiveChunkCounters []uint8        // counts the effective  chunk references in intermediate chunks. key is the chunk level.
	maxChildrenChunks      uint8          // maximum number of chunk references in intermediate chunks.
	replicaPutter          storage.Putter // putter to save dispersed replicas of the root chunk
}

func NewHashTrieWriter(
	ctx context.Context,
	refLen int,
	rParams redundancy.RedundancyParams,
	pipelineFn pipeline.PipelineFunc,
	replicaPutter storage.Putter,
) pipeline.ChainWriter {
	h := &hashTrieWriter{
		ctx:                    ctx,
		refSize:                refLen,
		cursors:                make([]int, 9),
		buffer:                 make([]byte, swarm.ChunkWithSpanSize*9*2), // double size as temp workaround for weak calculation of needed buffer space
		rParams:                rParams,
		pipelineFn:             pipelineFn,
		chunkCounters:          make([]uint8, 9),
		effectiveChunkCounters: make([]uint8, 9),
		maxChildrenChunks:      uint8(rParams.MaxShards() + rParams.Parities(rParams.MaxShards())),
		replicaPutter:          replicas.NewPutter(replicaPutter),
	}
	h.parityChunkFn = func(level int, span, address []byte) error {
		return h.writeToIntermediateLevel(level, true, span, address, []byte{})
	}

	return h
}

// accepts writes of hashes from the previous writer in the chain, by definition these writes
// are on level 1
func (h *hashTrieWriter) ChainWrite(p *pipeline.PipeWriteArgs) error {
	oneRef := h.refSize + swarm.SpanSize
	l := len(p.Span) + len(p.Ref) + len(p.Key)
	if l%oneRef != 0 || l == 0 {
		return errInconsistentRefs
	}
	if h.full {
		return errTrieFull
	}
	if h.rParams.Level() == redundancy.NONE {
		return h.writeToIntermediateLevel(1, false, p.Span, p.Ref, p.Key)
	} else {
		return h.writeToDataLevel(p.Span, p.Ref, p.Key, p.Data)
	}
}

func (h *hashTrieWriter) writeToIntermediateLevel(level int, parityChunk bool, span, ref, key []byte) error {
	copy(h.buffer[h.cursors[level]:h.cursors[level]+len(span)], span)
	h.cursors[level] += len(span)
	copy(h.buffer[h.cursors[level]:h.cursors[level]+len(ref)], ref)
	h.cursors[level] += len(ref)
	copy(h.buffer[h.cursors[level]:h.cursors[level]+len(key)], key)
	h.cursors[level] += len(key)

	// update counters
	if !parityChunk {
		h.effectiveChunkCounters[level]++
	}
	h.chunkCounters[level]++
	if h.chunkCounters[level] == h.maxChildrenChunks {
		// at this point the erasure coded chunks have been written
		err := h.wrapFullLevel(level)
		return err
	}
	return nil
}

// writeToDataLevel caches data chunks and call writeToIntermediateLevel
func (h *hashTrieWriter) writeToDataLevel(span, ref, key, data []byte) error {
	// write dataChunks to the level above
	err := h.writeToIntermediateLevel(1, false, span, ref, key)
	if err != nil {
		return err
	}

	return h.rParams.ChunkWrite(0, data, h.parityChunkFn)
}

// wrapLevel wraps an existing level and writes the resulting hash to the following level
// then truncates the current level data by shifting the cursors.
// Steps are performed in the following order:
//   - take all of the data in the current level
//   - break down span and hash data
//   - sum the span size, concatenate the hash to the buffer
//   - call the short pipeline with the span and the buffer
//   - get the hash that was created, append it one level above, and if necessary, wrap that level too
//   - remove already hashed data from buffer
//
// assumes that h.chunkCounters[level] has reached h.maxChildrenChunks at fullchunk
// or redundancy.Encode was called in case of rightmost chunks
func (h *hashTrieWriter) wrapFullLevel(level int) error {
	data := h.buffer[h.cursors[level+1]:h.cursors[level]]
	sp := uint64(0)
	var hashes []byte
	offset := 0
	for i := uint8(0); i < h.effectiveChunkCounters[level]; i++ {
		// sum up the spans of the level, then we need to bmt them and store it as a chunk
		// then write the chunk address to the next level up
		sp += binary.LittleEndian.Uint64(data[offset : offset+swarm.SpanSize])
		offset += +swarm.SpanSize
		hash := data[offset : offset+h.refSize]
		offset += h.refSize
		hashes = append(hashes, hash...)
	}
	parities := 0
	for offset < len(data) {
		// we do not add span of parity chunks to the common because that is gibberish
		offset += +swarm.SpanSize
		hash := data[offset : offset+swarm.HashSize] // parity reference has always hash length
		offset += swarm.HashSize
		hashes = append(hashes, hash...)
		parities++
	}
	spb := make([]byte, 8)
	binary.LittleEndian.PutUint64(spb, sp)
	if parities > 0 {
		redundancy.EncodeLevel(spb, h.rParams.Level())
	}
	hashes = append(spb, hashes...)
	writer := h.pipelineFn()
	args := pipeline.PipeWriteArgs{
		Data: hashes,
		Span: spb,
	}
	err := writer.ChainWrite(&args)
	if err != nil {
		return err
	}

	err = h.writeToIntermediateLevel(level+1, false, args.Span, args.Ref, args.Key)
	if err != nil {
		return err
	}

	err = h.rParams.ChunkWrite(level, args.Data, h.parityChunkFn)
	if err != nil {
		return err
	}

	// this "truncates" the current level that was wrapped
	// by setting the cursors to the cursors of one level above
	h.cursors[level] = h.cursors[level+1]
	h.chunkCounters[level], h.effectiveChunkCounters[level] = 0, 0
	if level+1 == 8 {
		h.full = true
	}
	return nil
}

// Sum returns the Swarm merkle-root content-addressed hash
// of an arbitrary-length binary data.
// The algorithm it uses is as follows:
//   - From level 1 till maxLevel 8, iterate:
//     -- If level data length equals 0 then continue to next level
//     -- If level data length equals 1 reference then carry over level data to next
//     -- If level data length is bigger than 1 reference then sum the level and
//     write the result to the next level
//   - Return the hash in level 8
//
// the cases are as follows:
//   - one hash in a given level, in which case we _do not_ perform a hashing operation, but just move
//     the hash to the next level, potentially resulting in a level wrap
//   - more than one hash, in which case we _do_ perform a hashing operation, appending the hash to
//     the next level
func (h *hashTrieWriter) Sum() ([]byte, error) {
	for i := 1; i < maxLevel; i++ {
		l := h.chunkCounters[i]
		switch {
		case l == 0:
			// level empty, continue to the next.
			continue
		case l == h.maxChildrenChunks:
			// this case is possible and necessary due to the carry over
			// in the next switch case statement. normal writes done
			// through writeToLevel will automatically wrap a full level.
			// erasure encoding call is not necessary since ElevateCarrierChunk solves that
			err := h.wrapFullLevel(i)
			if err != nil {
				return nil, err
			}
		case l == 1:
			// this cursor assignment basically means:
			// take the hash|span|key from this level, and append it to
			// the data of the next level. you may wonder how this works:
			// every time we sum a level, the sum gets written into the next level
			// and the level cursor gets set to the next level's cursor (see the
			// truncating at the end of wrapFullLevel). there might (or not) be
			// a hash at the next level, and the cursor of the next level is
			// necessarily _smaller_ than the cursor of this level, so in fact what
			// happens is that due to the shifting of the cursors, the data of this
			// level will appear to be concatenated with the data of the next level.
			// we therefore get a "carry-over" behavior between intermediate levels
			// that might or might not have data. the eventual result is that the last
			// hash generated will always be carried over to the last level (8), then returned.
			h.cursors[i+1] = h.cursors[i]
			// replace cached chunk to the level as well
			err := h.rParams.ElevateCarrierChunk(i-1, h.parityChunkFn)
			if err != nil {
				return nil, err
			}
			// update counters, subtracting from current level is not necessary
			h.effectiveChunkCounters[i+1]++
			h.chunkCounters[i+1]++
		default:
			// call erasure encoding before writing the last chunk on the level
			err := h.rParams.Encode(i-1, h.parityChunkFn)
			if err != nil {
				return nil, err
			}
			// more than 0 but smaller than chunk size - wrap the level to the one above it
			err = h.wrapFullLevel(i)
			if err != nil {
				return nil, err
			}
		}
	}
	levelLen := h.chunkCounters[maxLevel]
	if levelLen != 1 {
		return nil, errInconsistentRefs
	}

	// return the hash in the highest level, that's all we need
	data := h.buffer[0:h.cursors[maxLevel]]
	rootHash := data[swarm.SpanSize:]

	// save disperse replicas of the root chunk
	if h.rParams.Level() != redundancy.NONE {
		rootData, err := h.rParams.GetRootData()
		if err != nil {
			return nil, err
		}
		err = h.replicaPutter.Put(h.ctx, swarm.NewChunk(swarm.NewAddress(rootHash[:swarm.HashSize]), rootData))
		if err != nil {
			return nil, fmt.Errorf("hashtrie: cannot put dispersed replica %s", err.Error())
		}
	}
	return rootHash, nil
}
