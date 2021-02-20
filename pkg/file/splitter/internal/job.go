// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/encryption"
	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/sctx"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/tags"
	"golang.org/x/crypto/sha3"
)

type Putter interface {
	Put(context.Context, swarm.Chunk) ([]bool, error)
}

// maximum amount of file tree levels this file hasher component can handle
// (128 ^ (9 - 1)) * 4096 = 295147905179352825856 bytes
const levelBufferLimit = 9

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
	putter     Putter
	spanLength int64  // target length of data
	length     int64  // number of bytes written to the data level of the hasher
	sumCounts  []int  // number of sums performed, indexed per level
	cursors    []int  // section write position, indexed per level
	buffer     []byte // keeps data and hashes, indexed by cursors
	tag        *tags.Tag
	toEncrypt  bool // to encryrpt the chunks or not
	refSize    int64
}

// NewSimpleSplitterJob creates a new SimpleSplitterJob.
//
// The spanLength is the length of the data that will be written.
func NewSimpleSplitterJob(ctx context.Context, putter Putter, spanLength int64, toEncrypt bool) *SimpleSplitterJob {
	hashSize := swarm.HashSize
	refSize := int64(hashSize)
	if toEncrypt {
		refSize += encryption.KeyLength
	}

	return &SimpleSplitterJob{
		ctx:        ctx,
		putter:     putter,
		spanLength: spanLength,
		sumCounts:  make([]int, levelBufferLimit),
		cursors:    make([]int, levelBufferLimit),
		buffer:     make([]byte, swarm.ChunkWithSpanSize*levelBufferLimit*2), // double size as temp workaround for weak calculation of needed buffer space
		tag:        sctx.GetTag(ctx),
		toEncrypt:  toEncrypt,
		refSize:    refSize,
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

	var chunkData []byte

	head := make([]byte, swarm.SpanSize)
	binary.LittleEndian.PutUint64(head, uint64(span))
	tail := s.buffer[s.cursors[lvl+1]:s.cursors[lvl]]
	chunkData = append(head, tail...)
	err := s.incrTag(tags.StateSplit)
	if err != nil {
		return nil, err
	}
	c := chunkData
	var encryptionKey encryption.Key

	if s.toEncrypt {
		var err error
		c, encryptionKey, err = s.encryptChunkData(chunkData)
		if err != nil {
			return nil, err
		}
	}

	ch, err := cac.NewWithDataSpan(c)
	if err != nil {
		return nil, err
	}

	// Add tag to the chunk if tag is valid
	if s.tag != nil {
		ch = ch.WithTagID(s.tag.Uid)
	}

	seen, err := s.putter.Put(s.ctx, ch)
	if err != nil {
		return nil, err
	} else if len(seen) > 0 && seen[0] {
		err = s.incrTag(tags.StateSeen)
		if err != nil {
			return nil, err
		}
	}

	err = s.incrTag(tags.StateStored)
	if err != nil {
		return nil, err
	}

	return append(ch.Address().Bytes(), encryptionKey...), nil
}

// digest returns the calculated digest after a Sum call.
//
// The hash returned is the hash in the first section index of the work buffer
// this will be the root hash when all recursive sums have completed.
//
// The method does not check that the final hash actually has been written, so
// timing is the responsibility of the caller.
func (s *SimpleSplitterJob) digest() []byte {
	if s.toEncrypt {
		return s.buffer[:swarm.SectionSize*2]
	} else {
		return s.buffer[:swarm.SectionSize]
	}
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

func (s *SimpleSplitterJob) encryptChunkData(chunkData []byte) ([]byte, encryption.Key, error) {
	if len(chunkData) < 8 {
		return nil, nil, fmt.Errorf("invalid data, min length 8 got %v", len(chunkData))
	}

	key, encryptedSpan, encryptedData, err := s.encrypt(chunkData)
	if err != nil {
		return nil, nil, err
	}
	c := make([]byte, len(encryptedSpan)+len(encryptedData))
	copy(c[:8], encryptedSpan)
	copy(c[8:], encryptedData)
	return c, key, nil
}

func (s *SimpleSplitterJob) encrypt(chunkData []byte) (encryption.Key, []byte, []byte, error) {
	key := encryption.GenerateRandomKey(encryption.KeyLength)
	encryptedSpan, err := s.newSpanEncryption(key).Encrypt(chunkData[:8])
	if err != nil {
		return nil, nil, nil, err
	}
	encryptedData, err := s.newDataEncryption(key).Encrypt(chunkData[8:])
	if err != nil {
		return nil, nil, nil, err
	}
	return key, encryptedSpan, encryptedData, nil
}

func (s *SimpleSplitterJob) newSpanEncryption(key encryption.Key) encryption.Interface {
	return encryption.New(key, 0, uint32(swarm.ChunkSize/s.refSize), sha3.NewLegacyKeccak256)
}

func (s *SimpleSplitterJob) newDataEncryption(key encryption.Key) encryption.Interface {
	return encryption.New(key, int(swarm.ChunkSize), 0, sha3.NewLegacyKeccak256)
}

func (s *SimpleSplitterJob) incrTag(state tags.State) error {
	if s.tag != nil {
		return s.tag.Inc(state)
	}
	return nil
}
