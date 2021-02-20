// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hashtrie

import (
	"encoding/binary"
	"errors"

	"github.com/ethersphere/bee/pkg/file/pipeline"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	errInconsistentRefs = errors.New("inconsistent references")
	errTrieFull         = errors.New("trie full")
)

const maxLevel = 8

type hashTrieWriter struct {
	branching  int
	chunkSize  int
	refSize    int
	fullChunk  int    // full chunk size in terms of the data represented in the buffer (span+refsize)
	cursors    []int  // level cursors, key is level. level 0 is data level and is not represented in this package. writes always start at level 1. higher levels will always have LOWER cursor values.
	buffer     []byte // keeps all level data
	full       bool   // indicates whether the trie is full. currently we support (128^7)*4096 = 2305843009213693952 bytes
	pipelineFn pipeline.PipelineFunc
}

func NewHashTrieWriter(chunkSize, branching, refLen int, pipelineFn pipeline.PipelineFunc) pipeline.ChainWriter {
	return &hashTrieWriter{
		cursors:    make([]int, 9),
		buffer:     make([]byte, swarm.ChunkWithSpanSize*9*2), // double size as temp workaround for weak calculation of needed buffer space
		branching:  branching,
		chunkSize:  chunkSize,
		refSize:    refLen,
		fullChunk:  (refLen + swarm.SpanSize) * branching,
		pipelineFn: pipelineFn,
	}
}

// accepts writes of hashes from the previous writer in the chain, by definition these writes
// are on level 1
func (h *hashTrieWriter) ChainWrite(p *pipeline.PipeWriteArgs) error {
	oneRef := h.refSize + swarm.SpanSize
	l := len(p.Span) + len(p.Ref) + len(p.Key)
	if l%oneRef != 0 {
		return errInconsistentRefs
	}
	if h.full {
		return errTrieFull
	}
	return h.writeToLevel(1, p.Span, p.Ref, p.Key)
}

func (h *hashTrieWriter) writeToLevel(level int, span, ref, key []byte) error {
	copy(h.buffer[h.cursors[level]:h.cursors[level]+len(span)], span)
	h.cursors[level] += len(span)
	copy(h.buffer[h.cursors[level]:h.cursors[level]+len(ref)], ref)
	h.cursors[level] += len(ref)
	copy(h.buffer[h.cursors[level]:h.cursors[level]+len(key)], key)
	h.cursors[level] += len(key)
	howLong := (h.refSize + swarm.SpanSize) * h.branching

	if h.levelSize(level) == howLong {
		return h.wrapFullLevel(level)
	}
	return nil
}

// wrapLevel wraps an existing level and writes the resulting hash to the following level
// then truncates the current level data by shifting the cursors.
// Steps are performed in the following order:
//	 - take all of the data in the current level
//	 - break down span and hash data
//	 - sum the span size, concatenate the hash to the buffer
//	 - call the short pipeline with the span and the buffer
//	 - get the hash that was created, append it one level above, and if necessary, wrap that level too
//	 - remove already hashed data from buffer
// assumes that the function has been called when refsize+span*branching has been reached
func (h *hashTrieWriter) wrapFullLevel(level int) error {
	data := h.buffer[h.cursors[level+1]:h.cursors[level]]
	sp := uint64(0)
	var hashes []byte
	for i := 0; i < len(data); i += h.refSize + 8 {
		// sum up the spans of the level, then we need to bmt them and store it as a chunk
		// then write the chunk address to the next level up
		sp += binary.LittleEndian.Uint64(data[i : i+8])
		hash := data[i+8 : i+h.refSize+8]
		hashes = append(hashes, hash...)
	}
	spb := make([]byte, 8)
	binary.LittleEndian.PutUint64(spb, sp)
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
	err = h.writeToLevel(level+1, args.Span, args.Ref, args.Key)
	if err != nil {
		return err
	}

	// this "truncates" the current level that was wrapped
	// by setting the cursors to the cursors of one level above
	h.cursors[level] = h.cursors[level+1]
	if level+1 == 8 {
		h.full = true
	}
	return nil
}

func (h *hashTrieWriter) levelSize(level int) int {
	if level == 8 {
		return h.cursors[level]
	}
	return h.cursors[level] - h.cursors[level+1]
}

// Sum returns the Swarm merkle-root content-addressed hash
// of an arbitrary-length binary data.
// The algorithm it uses is as follows:
//	- From level 1 till maxLevel 8, iterate:
//		-	If level data length equals 0 then continue to next level
//		-	If level data length equals 1 reference then carry over level data to next
//		-	If level data length is bigger than 1 reference then sum the level and
//			write the result to the next level
//	- Return the hash in level 8
// the cases are as follows:
//	- one hash in a given level, in which case we _do not_ perform a hashing operation, but just move
//		the hash to the next level, potentially resulting in a level wrap
//	- more than one hash, in which case we _do_ perform a hashing operation, appending the hash to
//		the next level
func (h *hashTrieWriter) Sum() ([]byte, error) {
	oneRef := h.refSize + swarm.SpanSize
	for i := 1; i < maxLevel; i++ {
		l := h.levelSize(i)
		if l%oneRef != 0 {
			return nil, errInconsistentRefs
		}
		switch {
		case l == 0:
			// level empty, continue to the next.
			continue
		case l == h.fullChunk:
			// this case is possible and necessary due to the carry over
			// in the next switch case statement. normal writes done
			// through writeToLevel will automatically wrap a full level.
			err := h.wrapFullLevel(i)
			if err != nil {
				return nil, err
			}
		case l == oneRef:
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
		default:
			// more than 0 but smaller than chunk size - wrap the level to the one above it
			err := h.wrapFullLevel(i)
			if err != nil {
				return nil, err
			}
		}
	}
	levelLen := h.levelSize(8)
	if levelLen != oneRef {
		return nil, errInconsistentRefs
	}

	// return the hash in the highest level, that's all we need
	data := h.buffer[0:h.cursors[8]]
	return data[8:], nil
}
