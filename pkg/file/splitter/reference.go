package hasher

import (
	"github.com/ethersphere/swarm/file"
)

// ReferenceHasher is the source-of-truth implementation of the swarm file hashing algorithm
type ReferenceHasher struct {
	params  *treeParams
	cursors []int              // section write position, indexed per level
	length  int                // number of bytes written to the data level of the hasher
	buffer  []byte             // keeps data and hashes, indexed by cursors
	counts  []int              // number of sums performed, indexed per level
	hasher  file.SectionWriter // underlying hasher
}

// NewReferenceHasher constructs and returns a new ReferenceHasher
// This implementation is limited to a tree of 9 levels, where level 0 is the data level
// With 32 section size and 128 branches (i.e. unencrypted, non erasure-coded content) this means
// a capacity of 4096 bytes * (128^(9-1)) ~ 295.148 * (10^18) bytes
func NewReferenceHasher(params *treeParams) *ReferenceHasher {
	// TODO: remove when bmt interface is amended
	h := params.GetWriter()
	return &ReferenceHasher{
		params:  params,
		cursors: make([]int, 9),
		counts:  make([]int, 9),
		buffer:  make([]byte, params.ChunkSize*9),
		hasher:  h,
	}
}

// Hash computes and returns the root hash of arbitrary data
func (r *ReferenceHasher) Hash(data []byte) []byte {
	l := r.params.ChunkSize
	for i := 0; i < len(data); i += r.params.ChunkSize {
		if len(data)-i < r.params.ChunkSize {
			l = len(data) - i
		}
		r.update(0, data[i:i+l])
	}

	// if we didn't end on a chunk boundary we need to hash remaining chunks first
	r.hashUnfinished()

	// if the already hashed parts tree is balanced
	r.moveDanglingChunk()

	return r.digest()
}

// write to the data buffer on the specified level
// calls sum if chunk boundary is reached and recursively calls this function for the next level with the acquired bmt hash
// adjusts cursors accordingly
func (r *ReferenceHasher) update(lvl int, data []byte) {
	if lvl == 0 {
		r.length += len(data)
	}
	copy(r.buffer[r.cursors[lvl]:r.cursors[lvl]+len(data)], data)
	r.cursors[lvl] += len(data)
	if r.cursors[lvl]-r.cursors[lvl+1] == r.params.ChunkSize {
		ref := r.sum(lvl)
		r.update(lvl+1, ref)
		r.cursors[lvl] = r.cursors[lvl+1]
	}
}

// calculates and returns the bmt sum of the last written data on the level
func (r *ReferenceHasher) sum(lvl int) []byte {
	r.counts[lvl]++
	spanSize := r.params.Spans[lvl] * r.params.ChunkSize
	span := (r.length-1)%spanSize + 1

	sizeToSum := r.cursors[lvl] - r.cursors[lvl+1]

	r.hasher.Reset()
	r.hasher.SetSpan(span)
	r.hasher.Write(r.buffer[r.cursors[lvl+1] : r.cursors[lvl+1]+sizeToSum])
	ref := r.hasher.Sum(nil)
	return ref
}

// called after all data has been written
// sums the final chunks of each level
// skips intermediate levels that end on span boundary
func (r *ReferenceHasher) digest() []byte {

	// the first section of the buffer will hold the root hash
	return r.buffer[:r.params.SectionSize]
}

// hashes the remaining unhashed chunks at the end of each level
func (r *ReferenceHasher) hashUnfinished() {
	if r.length%r.params.ChunkSize != 0 {
		ref := r.sum(0)
		copy(r.buffer[r.cursors[1]:], ref)
		r.cursors[1] += len(ref)
		r.cursors[0] = r.cursors[1]
	}
}

// in case of a balanced tree this method concatenates the reference to the single reference
// at the highest level of the tree.
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
func (r *ReferenceHasher) moveDanglingChunk() {

	// calculate the total number of levels needed to represent the data (including the data level)
	targetLevel := getLevelsFromLength(r.length, r.params.SectionSize, r.params.Branches)

	// sum every intermediate level and write to the level above it
	for i := 1; i < targetLevel; i++ {

		// and if there is a single reference outside a balanced tree on this level
		// don't hash it again but pass it on to the next level
		if r.counts[i] > 0 {
			// TODO: simplify if possible
			if r.counts[i-1]-r.params.Spans[targetLevel-1-i] <= 1 {
				r.cursors[i+1] = r.cursors[i]
				r.cursors[i] = r.cursors[i-1]
				continue
			}
		}

		ref := r.sum(i)
		copy(r.buffer[r.cursors[i+1]:], ref)
		r.cursors[i+1] += len(ref)
		r.cursors[i] = r.cursors[i+1]
	}
}
