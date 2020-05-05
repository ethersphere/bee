package internal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/ethersphere/bee/pkg/logging"
	"github.com/ethersphere/bee/pkg/file"
)

// ReferenceHasher is the source-of-truth implementation of the swarm file hashing algorithm
type SimpleSplitterJob struct {
	ctx context.Context
	cursors []int              // section write position, indexed per level
	spanLength int64	   // target length of data
	length  int                // number of bytes written to the data level of the hasher
	buffer  []byte             // keeps data and hashes, indexed by cursors
	counts  []int              // number of sums performed, indexed per level
	dataC   chan []byte
	doneC   chan struct{}
	closeDoneOnce sync.Once     // make sure done channel is closed only once
	resultC	chan []byte
	err error
	logger        logging.Logger
}

func NewSimpleSplitterJob(ctx context.Context, store storage.Storer, spanLength int64) *SimpleSplitterJob {
	j := &SimpleSplitterJob{
		ctx: ctx,
		cursors: make([]int, 9),
		spanLength: spanLength,
		counts:  make([]int, 9),
		buffer:  make([]byte, swarm.ChunkSize*9),
		dataC:   make(chan []byte),
		doneC:   make(chan struct{}),
		resultC: make(chan []byte),
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
		close(j.dataC)
		j.closeDone()
	}()

	return j
}

func (j *SimpleSplitterJob) start() error {
	var total int64
	for {
		select {
		case <-j.ctx.Done():
			return j.ctx.Err()
		case <-j.doneC:
			if j.spanLength > 0 && total != j.spanLength {
				return file.NewAbortError(errors.New("file write aborted"))
			}
		case d := <-j.dataC:
			j.update(0, d)
			total += int64(len(d))
			if total == j.spanLength {
				j.logger.Tracef("file write done for context %v, length %d bytes", j.ctx, total)
				break
			}
		}
	}

	j.err = errors.New("Write called after Sum")

	if total > swarm.ChunkSize {
		j.hashUnfinished()
		j.moveDanglingChunk()
	}

	j.resultC <- j.digest()

	return nil
}

func (j *SimpleSplitterJob) Write(b []byte) (int, error) {
	if cap(b) > swarm.ChunkSize {
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

func (j *SimpleSplitterJob) Finish(l int64) (hash swarm.Address, err error) {
	return swarm.ZeroAddress, nil
}

//// write to the data buffer on the specified level
//// calls sum if chunk boundary is reached and recursively calls this function for the next level with the acquired bmt hash
//// adjusts cursors accordingly
func (s *SimpleSplitterJob) update(lvl int, data []byte) {
}
//	if lvl == 0 {
//		r.length += len(data)
//	}
//	copy(r.buffer[r.cursors[lvl]:r.cursors[lvl]+len(data)], data)
//	r.cursors[lvl] += len(data)
//	if r.cursors[lvl]-r.cursors[lvl+1] == r.params.ChunkSize {
//		ref := r.sum(lvl)
//		r.update(lvl+1, ref)
//		r.cursors[lvl] = r.cursors[lvl+1]
//	}
//}
//
//// calculates and returns the bmt sum of the last written data on the level
func (s *SimpleSplitterJob) sum(lvl int) []byte {
	return nil
}
//	r.counts[lvl]++
//	spanSize := r.params.Spans[lvl] * r.params.ChunkSize
//	span := (r.length-1)%spanSize + 1
//
//	sizeToSum := r.cursors[lvl] - r.cursors[lvl+1]
//
//	r.hasher.Reset()
//	r.hasher.SetSpan(span)
//	r.hasher.Write(r.buffer[r.cursors[lvl+1] : r.cursors[lvl+1]+sizeToSum])
//	ref := r.hasher.Sum(nil)
//	return ref
//}
//
//// called after all data has been written
//// sums the final chunks of each level
//// skips intermediate levels that end on span boundary
func (s *SimpleSplitterJob) digest() []byte {
	return nil
}
//
//	// the first section of the buffer will hold the root hash
//	return r.buffer[:r.params.SectionSize]
//}
//
//// hashes the remaining unhashed chunks at the end of each level
func (s *SimpleSplitterJob) hashUnfinished() {
}
//	if r.length%r.params.ChunkSize != 0 {
//		ref := r.sum(0)
//		copy(r.buffer[r.cursors[1]:], ref)
//		r.cursors[1] += len(ref)
//		r.cursors[0] = r.cursors[1]
//	}
//}
//
//// in case of a balanced tree this method concatenates the reference to the single reference
//// at the highest level of the tree.
////
//// Let F be full chunks (disregarding branching factor) and S be single references
//// in the following scenario:
////
////       S
////     F   F
////   F   F   F
//// F   F   F   F S
////
//// The result will be:
////
////       SS
////     F    F
////   F   F   F
//// F   F   F   F
////
//// After which the SS will be hashed to obtain the final root hash
func (s *SimpleSplitterJob) moveDanglingChunk() {
}
//
//	// calculate the total number of levels needed to represent the data (including the data level)
//	targetLevel := getLevelsFromLength(r.length, r.params.SectionSize, r.params.Branches)
//
//	// sum every intermediate level and write to the level above it
//	for i := 1; i < targetLevel; i++ {
//
//		// and if there is a single reference outside a balanced tree on this level
//		// don't hash it again but pass it on to the next level
//		if r.counts[i] > 0 {
//			// TODO: simplify if possible
//			if r.counts[i-1]-r.params.Spans[targetLevel-1-i] <= 1 {
//				r.cursors[i+1] = r.cursors[i]
//				r.cursors[i] = r.cursors[i-1]
//				continue
//			}
//		}
//
//		ref := r.sum(i)
//		copy(r.buffer[r.cursors[i+1]:], ref)
//		r.cursors[i+1] += len(ref)
//		r.cursors[i] = r.cursors[i+1]
//	}
//}

// closeDone, for purpose readability, wraps the sync.Once execution of closing the doneC channel
func (j *SimpleSplitterJob) closeDone() {
	j.closeDoneOnce.Do(func() {
		close(j.doneC)
	})
}
