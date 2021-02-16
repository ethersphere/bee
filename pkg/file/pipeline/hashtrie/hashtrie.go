// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package hashtrie

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/file/pipeline"
	"github.com/ethersphere/bee/pkg/swarm"
)

var errInconsistentRefs = errors.New("inconsistent reference lengths in level")

type hashTrieWriter struct {
	branching  int
	chunkSize  int
	refSize    int
	fullChunk  int    // full chunk size in terms of the data represented in the buffer (span+refsize)
	cursors    []int  // level cursors, key is level. level 0 is data level
	buffer     []byte // keeps all level data
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
	if len(p.Ref) == 0 {
		panic(0)
	}
	oneRef := h.refSize + swarm.SpanSize
	l := len(p.Span) + len(p.Ref) + len(p.Key)
	if l%oneRef != 0 {
		return errInconsistentRefs
	}
	return h.writeToLevel(1, p.Span, p.Ref, p.Key)
}

func (h *hashTrieWriter) writeToLevel(level int, span, ref, key []byte) error {
	fmt.Println("write to level", level, "span", span, "ref", ref, "cur", h.cursors[level])
	copy(h.buffer[h.cursors[level]:h.cursors[level]+len(span)], span)
	h.cursors[level] += len(span)
	copy(h.buffer[h.cursors[level]:h.cursors[level]+len(ref)], ref)
	h.cursors[level] += len(ref)
	copy(h.buffer[h.cursors[level]:h.cursors[level]+len(key)], key)
	h.cursors[level] += len(key)
	//if h.cursors[level+1] != 0 && h.cursors[level] > h.cursors[level+1] {
	//fmt.Println(h.cursors)
	//panic("overwrite")
	//}
	fmt.Println("cur", h.cursors[level])
	howLong := (h.refSize + swarm.SpanSize) * h.branching

	if h.levelSize(level) == howLong {
		fmt.Println("wrap full level", level, howLong)
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
	fmt.Println("wrap full, level", level)
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
	return nil
}

func (h *hashTrieWriter) levelSize(level int) int {
	if level == 8 {
		return h.cursors[level]
	}
	return h.cursors[level] - h.cursors[level+1]
}

func (h *hashTrieWriter) Sum() ([]byte, error) {
	oneRef := h.refSize + swarm.SpanSize
	maxLevel := 8
	// look from the top down, to look for the highest hash of a balanced tree
	// then, whatever is in the levels below that is necessarily unbalanced,
	// so, we'd like to reduce those levels to one hash, then wrap it together
	// with the balanced tree hash, to produce the root chunk.
	// the cases are as follows:
	//	- one hash in a given level, in which case we _do not_ perform a hashing operation, but just move
	//		the hash to the next level, potentially resulting in a level wrap
	//	- more than one hash, in which case we _do_ perform a hashing operation, appending the hash to
	//		the next level.
	fmt.Println("sum")
	for i := 1; i < maxLevel; i++ {
		l := h.levelSize(i)
		fmt.Println("level size", i, l, "cursors", h.cursors)
		if l%oneRef != 0 {
			return nil, errInconsistentRefs
		}
		switch {
		case l == 0:
			fmt.Println("hoist, empty level")
			continue
		case l == h.fullChunk:
			fmt.Println("hoist, full chunk")
			err := h.wrapFullLevel(i)
			if err != nil {
				return nil, err
			}
			h.cursors[i] = h.cursors[i-1]
		case l == oneRef:
			// this cursor assignment basically means:
			// take the hash|span|key from this level, and append it to
			// the data of the next level.
			h.cursors[i+1] = h.cursors[i]
			//h.cursors[i] = h.cursors[i-1]
			fmt.Println("hoist, one ref", h.cursors)
		default:
			fmt.Println("hoist, some stuff")
			// more than 0 but smaller than chunk size - wrap the level to the one above it
			err := h.wrapFullLevel(i)
			if err != nil {
				return nil, err
			}
		}
	}
	tlen := h.levelSize(8)
	fmt.Println(h.cursors, tlen)
	// take the hash in the highest level, that's all we need
	data := h.buffer[0:h.cursors[8]]
	fmt.Println(data)
	if tlen%oneRef != 0 {
		return nil, errInconsistentRefs
	}
	if tlen == oneRef {
		return data[8:], nil
	}
	panic(tlen)
}
