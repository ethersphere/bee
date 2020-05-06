// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"context"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"sync"

	"github.com/ethersphere/bee/pkg/file"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bmt"
	bmtlegacy "github.com/ethersphere/bmt/legacy"
	"golang.org/x/crypto/sha3"
)

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
	ctx           context.Context
	store         storage.Storer
	spanLength    int64         // target length of data
	length        int64         // number of bytes written to the data level of the hasher
	sumCounts     []int         // number of sums performed, indexed per level
	cursors       []int         // section write position, indexed per level
	hasher        bmt.Hash      // underlying hasher used for hashing the tree
	buffer        []byte        // keeps data and hashes, indexed by cursors
	dataC         chan []byte   // receives data in the processing thread from Write calls
	doneC         chan struct{} // closed when last write has been performed and/or sum is called
	resultC       chan []byte   // passes result hash from the processing thread to the Sum call
	closeDoneOnce sync.Once     // make sure done channel is closed only once
	err           error
	logger        logging.Logger
}

// NewSimpleSplitterJob creates a new SimpleSplitterJob.
//
// The spanLength is the length of the data that will be written.
func NewSimpleSplitterJob(ctx context.Context, store storage.Storer, spanLength int64) *SimpleSplitterJob {

	p := bmtlegacy.NewTreePool(hashFunc, swarm.Branches, bmtlegacy.PoolSize)
	j := &SimpleSplitterJob{
		ctx:        ctx,
		store:      store,
		spanLength: spanLength,
		sumCounts:     make([]int, 9),
		cursors:    make([]int, 9),
		hasher:     bmtlegacy.New(p),
		buffer:     make([]byte, swarm.ChunkSize*9),
		dataC:      make(chan []byte),
		doneC:      make(chan struct{}),
		resultC:    make(chan []byte),
		logger:     logging.New(os.Stderr, 6),
	}

	go func() {
		err := j.start()
		if err != nil {
			if err != io.EOF {
				j.logger.Errorf("simple splitter chunk split job with context %v fail: %v", j.ctx, err)
			} else {
				j.logger.Tracef("simple splitter split job with context %v eof", j.ctx)
			}
		}
		j.err = err
		j.closeDone()
		close(j.dataC)
	}()

	return j
}

// start initiates a loop that consumes written data from the caller and passes it to the
// tree building process.
//
// When write is done it calls the post-processing functions for the tree
// and passes the digest to the Sum call through the result channel.
//
// It also handles context aborts and catches premature calls to Sum.
func (j *SimpleSplitterJob) start() error {
	var total int64

	// TODO: put in separate function to avoid outer label
OUTER:
	for {
		select {
		case <-j.ctx.Done():
			return j.ctx.Err()
		case <-j.doneC:
			if j.spanLength > 0 && total != j.spanLength {
				return file.NewAbortError(errors.New("file write aborted"))
			}
		case d := <-j.dataC:
			j.writeToLevel(0, d)
			total += int64(len(d))
			if total == j.spanLength {
				j.err = errors.New("Write called after Sum")
				j.logger.Tracef("last write %d done for context %v, total length %d bytes", len(d), j.ctx, total)
				break OUTER
			}
		}
	}

	j.hashUnfinished()

	if total > swarm.ChunkSize {
		j.moveDanglingChunk()
	}

	select {
	case j.resultC <- j.digest():
	case <-j.ctx.Done():
		return j.ctx.Err()
	}

	return nil
}

// Write adds data to the file splitter process.
func (j *SimpleSplitterJob) Write(b []byte) (int, error) {
	if len(b) > swarm.ChunkSize {
		return 0, fmt.Errorf("Write must be called with a maximum of %d bytes", swarm.ChunkSize)
	}

	// Write assumes that doneC will be closed on any state of abortion
	select {
	case <-j.doneC:
		return 0, j.err
	case j.dataC <- b:
	}
	return len(b), nil
}

// Sum returns the hash of the file hasher job once it is calculated.
func (j *SimpleSplitterJob) Sum(b []byte) []byte {

	// doneC signals writes are completed
	j.closeDone()

	// wait for result of timeout event
	// if times out result will be nil
	// the error state will not be reported, so important that Write() handles any edge case that may lead to error
	var result []byte
	select {
	case result = <-j.resultC:
	case <-j.ctx.Done():
	}
	return result
}

// writeToLevel writes to the data buffer on the specified level.
// It calls sum if chunk boundary is reached and recursively calls this function for
// the next level with the acquired bmt hash
//
// It adjusts the relevant levels' cursors accordingly.
func (s *SimpleSplitterJob) writeToLevel(lvl int, data []byte) {
	if lvl == 0 {
		s.length += int64(len(data))
	}
	copy(s.buffer[s.cursors[lvl]:s.cursors[lvl]+len(data)], data)
	s.cursors[lvl] += len(data)
	if s.cursors[lvl]-s.cursors[lvl+1] == swarm.ChunkSize {
		ref := s.sumLevel(lvl)
		s.writeToLevel(lvl+1, ref)
		s.cursors[lvl] = s.cursors[lvl+1]
	}
}

// sumLevel calculates and returns the bmt sum of the last written data on the level.
//
// TODO: error handling on store write fail
func (s *SimpleSplitterJob) sumLevel(lvl int) []byte {
	s.sumCounts[lvl]++
	spanSize := file.Spans[lvl] * swarm.ChunkSize
	span := (s.length-1)%spanSize + 1

	sizeToSum := s.cursors[lvl] - s.cursors[lvl+1]

	s.hasher.Reset()
	err := s.hasher.SetSpan(span)
	if err != nil {
		return nil
	}
	_, err = s.hasher.Write(s.buffer[s.cursors[lvl+1] : s.cursors[lvl+1]+sizeToSum])
	if err != nil {
		return nil
	}
	ref := s.hasher.Sum(nil)
	addr := swarm.NewAddress(ref)
	ch := swarm.NewChunk(addr, s.buffer[s.cursors[lvl+1]:s.cursors[lvl]])
	_, err = s.store.Put(s.ctx, storage.ModePutUpload, ch)
	if err != nil {
		return nil
	}
	return ref
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
func (s *SimpleSplitterJob) hashUnfinished() {
	if s.length%swarm.ChunkSize != 0 {
		ref := s.sumLevel(0)
		copy(s.buffer[s.cursors[1]:], ref)
		s.cursors[1] += len(ref)
		s.cursors[0] = s.cursors[1]
	}
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
func (s *SimpleSplitterJob) moveDanglingChunk() {
	// calculate the total number of levels needed to represent the data (including the data level)
	targetLevel := file.GetLevelsFromLength(s.length, swarm.SectionSize, swarm.Branches)

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

		ref := s.sumLevel(i)
		copy(s.buffer[s.cursors[i+1]:], ref)
		s.cursors[i+1] += len(ref)
		s.cursors[i] = s.cursors[i+1]
	}
}

// closeDone, for purpose readability, wraps the sync.Once execution of closing the doneC channel
func (j *SimpleSplitterJob) closeDone() {
	j.closeDoneOnce.Do(func() {
		close(j.doneC)
	})
}
