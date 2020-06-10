// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bmt"
	bmtlegacy "github.com/ethersphere/bmt/legacy"
	"golang.org/x/crypto/sha3"
)

// maximum amount of file tree levels this file hasher component can handle
// (128 ^ (9 - 1)) * 4096 = 295147905179352825856 bytes
const levelBufferLimit = 9

// hashFunc is a hasher factory used by the bmt hasher
func hashFunc() hash.Hash {
	return sha3.NewLegacyKeccak256()
}

// SimpleSplitterJob encapsulated a single splitter operation, accepting blockwise
// writes of data whose length is defined in advance.
//
// After the job is constructed, Write must be called with up to ChunkSize byte slices
// until the full data length has been written. The Sum should be called which will
// return the SwarmHash of the data.
//
// Called Sum before the last Write, or Write after Sum has been called, may result in
// error and will may result in undefined result.
type SimpleSplitterJob struct {
	ctx        context.Context
	putter     storage.Putter
	spanLength int64    // target length of data
	length     int64    // number of bytes written to the data level of the hasher
	sumCounts  []int    // number of sums performed, indexed per level
	cursors    []int    // section write position, indexed per level
	hasher     bmt.Hash // underlying hasher used for hashing the tree
	buffer     []byte   // keeps data and hashes, indexed by cursors
}

// NewSimpleSplitterJob creates a new SimpleSplitterJob.
//
// The spanLength is the length of the data that will be written.
func NewSimpleSplitterJob(ctx context.Context, putter storage.Putter, spanLength int64) *SimpleSplitterJob {
	p := bmtlegacy.NewTreePool(hashFunc, swarm.Branches, bmtlegacy.PoolSize)
	return &SimpleSplitterJob{
		ctx:        ctx,
		putter:     putter,
		spanLength: spanLength,
		sumCounts:  make([]int, levelBufferLimit),
		cursors:    make([]int, levelBufferLimit),
		hasher:     bmtlegacy.New(p),
		buffer:     make([]byte, file.ChunkWithLengthSize*levelBufferLimit*2), // double size as temp workaround for weak calculation of needed buffer space
	}
}

// Write adds data to the file splitter.
func (j *SimpleSplitterJob) Write(b []byte) (int, error) {
	if len(b) > swarm.ChunkSize {
		return 0, fmt.Errorf("Write must be called with a maximum of %d bytes", swarm.ChunkSize)
	}
	j.length += int64(len(b))
	if j.length > j.spanLength {
		return 0, errors.New("write past span length")
	}

	err := j.writeToLevel(0, b)
	if err != nil {
		return 0, err
	}
	if j.length == j.spanLength {
		err := j.hashUnfinished()
		if err != nil {
			return 0, file.NewHashError(err)
		}
		err = j.moveDanglingChunk()
		if err != nil {
			return 0, file.NewHashError(err)
		}

	}
	return len(b), nil
}

// Sum returns the Swarm hash of the data.
func (j *SimpleSplitterJob) Sum(b []byte) []byte {
	return j.digest()
}

// writeToLevel writes to the data buffer on the specified level.
// It calls sum if chunk boundary is reached and recursively calls this function for
// the next level with the acquired bmt hash
//
// It adjusts the relevant levels' cursors accordingly.
func (s *SimpleSplitterJob) writeToLevel(lvl int, data []byte) error {
	copy(s.buffer[s.cursors[lvl]:s.cursors[lvl]+len(data)], data)
	s.cursors[lvl] += len(data)
	if s.cursors[lvl]-s.cursors[lvl+1] == swarm.ChunkSize {
		ref, err := s.sumLevel(lvl)
		if err != nil {
			return err
		}
		err = s.writeToLevel(lvl+1, ref)
		if err != nil {
			return err
		}
		s.cursors[lvl] = s.cursors[lvl+1]
	}
	return nil
}

// sumLevel calculates and returns the bmt sum of the last written data on the level.
//
// TODO: error handling on store write fail
func (s *SimpleSplitterJob) sumLevel(lvl int) ([]byte, error) {
	s.sumCounts[lvl]++
	spanSize := file.Spans[lvl] * swarm.ChunkSize
	span := (s.length-1)%spanSize + 1

	sizeToSum := s.cursors[lvl] - s.cursors[lvl+1]

	// perform hashing
	s.hasher.Reset()
	err := s.hasher.SetSpan(span)
	if err != nil {
		return nil, err
	}
	_, err = s.hasher.Write(s.buffer[s.cursors[lvl+1] : s.cursors[lvl+1]+sizeToSum])
	if err != nil {
		return nil, err
	}
	ref := s.hasher.Sum(nil)

	// assemble chunk and put in store
	addr := swarm.NewAddress(ref)
	head := make([]byte, 8)
	binary.LittleEndian.PutUint64(head, uint64(span))
	tail := s.buffer[s.cursors[lvl+1]:s.cursors[lvl]]
	chunkData := append(head, tail...)
	ch := swarm.NewChunk(addr, chunkData)
	_, err = s.putter.Put(s.ctx, storage.ModePutUpload, ch)
	if err != nil {
		return nil, err
	}

	return ref, nil
}

// digest returns the calculated digest after a Sum call.
//
// The hash returned is the hash in the first section index of the work buffer
// this will be the root hash when all recursive sums have completed.
//
// The method does not check that the final hash actually has been written, so
// timing is the responsibility of the caller.
func (s *SimpleSplitterJob) digest() []byte {
	return s.buffer[:swarm.SectionSize]
}

// hashUnfinished hasher the remaining unhashed chunks at the end of each level if
// write doesn't end on a chunk boundary.
func (s *SimpleSplitterJob) hashUnfinished() error {
	if s.length%swarm.ChunkSize != 0 {
		ref, err := s.sumLevel(0)
		if err != nil {
			return err
		}
		copy(s.buffer[s.cursors[1]:], ref)
		s.cursors[1] += len(ref)
		s.cursors[0] = s.cursors[1]
	}
	return nil
}

// moveDanglingChunk concatenates the reference to the single reference
// at the highest level of the tree in case of a balanced tree.
//
// Let F be full chunks (disregarding branching factor) and S be single references
// in the following scenario:
//
//       S
//     F   F
//   F   F   F
// F   F   F   F S
//
// The result will be:
//
//       SS
//     F    F
//   F   F   F
// F   F   F   F
//
// After which the SS will be hashed to obtain the final root hash
func (s *SimpleSplitterJob) moveDanglingChunk() error {
	// calculate the total number of levels needed to represent the data (including the data level)
	targetLevel := file.Levels(s.length, swarm.SectionSize, swarm.Branches)

	// sum every intermediate level and write to the level above it
	for i := 1; i < targetLevel; i++ {

		// and if there is a single reference outside a balanced tree on this level
		// don't hash it again but pass it on to the next level
		if s.sumCounts[i] > 0 {
			// TODO: simplify if possible
			if int64(s.sumCounts[i-1])-file.Spans[targetLevel-1-i] <= 1 {
				s.cursors[i+1] = s.cursors[i]
				s.cursors[i] = s.cursors[i-1]
				continue
			}
		}

		ref, err := s.sumLevel(i)
		if err != nil {
			return err
		}
		copy(s.buffer[s.cursors[i+1]:], ref)
		s.cursors[i+1] += len(ref)
		s.cursors[i] = s.cursors[i+1]
	}
	return nil
}
