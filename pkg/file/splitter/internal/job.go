package internal

import (
	"context"
	"io"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

// ReferenceHasher is the source-of-truth implementation of the swarm file hashing algorithm
type SimpleSplitterJob struct {
	cursors []int              // section write position, indexed per level
	length  int                // number of bytes written to the data level of the hasher
	buffer  []byte             // keeps data and hashes, indexed by cursors
	counts  []int              // number of sums performed, indexed per level
	reader io.ReadCloser
}

func NewSimpleSplitterJob(ctx context.Context, r io.ReadCloser, store storage.Storer) *SimpleSplitterJob {
	j := &SimpleSplitterJob{
		cursors: make([]int, 9),
		counts:  make([]int, 9),
		buffer:  make([]byte, swarm.ChunkSize*9),
		reader: r,
	}

	data := make([]byte, swarm.ChunkSize)
	go func() {
		var total int
		for {
			c, err := r.Read(data)
			// TODO: provide error to caller
			if err != nil {
				if err == io.EOF {
					break
				}
				return
			}
			j.update(0, data[:c])
			total += c
		}

		j.hashUnfinished()

		j.moveDanglingChunk()

		j.digest()
	}()
	return j
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
