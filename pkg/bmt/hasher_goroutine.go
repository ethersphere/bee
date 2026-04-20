// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt

import (
	"encoding/binary"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var _ Hasher = (*goroutineHasher)(nil)

// goroutineHasher is the goroutine-based BMT hasher implementation.
// It uses one goroutine per leaf section to hash sections concurrently,
// with atomic toggles to coordinate parent-node writes.
//
// The same hasher instance must not be called concurrently on more than one chunk.
// The same hasher instance is synchronously reusable after calling Reset.
type goroutineHasher struct {
	*goroutineConf
	bmt    *goroutineTree
	size   int
	pos    int
	result chan []byte
	errc   chan error
	span   []byte
}

func newGoroutineHasher() *goroutineHasher {
	gc := newGoroutineConf(nil, swarm.BmtBranches, 32)
	return &goroutineHasher{
		goroutineConf: gc,
		result:        make(chan []byte),
		errc:          make(chan error, 1),
		span:          make([]byte, SpanSize),
		bmt:           newGoroutineTree(gc.maxSize, gc.depth, gc.hasherFunc),
	}
}

func newGoroutinePrefixHasher(prefix []byte) *goroutineHasher {
	gc := newGoroutineConf(prefix, swarm.BmtBranches, 32)
	return &goroutineHasher{
		goroutineConf: gc,
		result:        make(chan []byte),
		errc:          make(chan error, 1),
		span:          make([]byte, SpanSize),
		bmt:           newGoroutineTree(gc.maxSize, gc.depth, gc.hasherFunc),
	}
}

// Capacity returns the maximum number of bytes this hasher can process.
func (h *goroutineHasher) Capacity() int { return h.maxSize }

// Size returns the digest size.
func (h *goroutineHasher) Size() int { return h.segmentSize }

// BlockSize returns the optimal write block size.
func (h *goroutineHasher) BlockSize() int { return 2 * h.segmentSize }

// Sum is the unsafe Hash wrapper.
func (h *goroutineHasher) Sum(b []byte) []byte { s, _ := h.Hash(b); return s }

// SetHeaderInt64 sets the span preamble from an int64.
func (h *goroutineHasher) SetHeaderInt64(length int64) {
	binary.LittleEndian.PutUint64(h.span, uint64(length))
}

// SetHeader copies the span preamble from the argument.
func (h *goroutineHasher) SetHeader(span []byte) { copy(h.span, span) }

// Write appends to the chunk buffer; each complete section triggers a goroutine-level hash.
func (h *goroutineHasher) Write(b []byte) (int, error) {
	l := len(b)
	maxVal := h.maxSize - h.size
	if l > maxVal {
		l = maxVal
	}
	copy(h.bmt.buffer[h.size:], b)
	secsize := 2 * h.segmentSize
	from := h.size / secsize
	h.size += l
	to := h.size / secsize
	if l == maxVal {
		to--
	}
	h.pos = to
	for i := from; i < to; i++ {
		go h.processSection(i, false)
	}
	return l, nil
}

// Hash returns the BMT root hash of the buffer written so far.
func (h *goroutineHasher) Hash(b []byte) ([]byte, error) {
	if h.size == 0 {
		return doHash(h.baseHasher(), h.span, h.zerohashes[h.depth])
	}
	copy(h.bmt.buffer[h.size:], zerosection)
	// write the last section with final flag set to true
	go h.processSection(h.pos, true)
	select {
	case result := <-h.result:
		return doHash(h.baseHasher(), h.span, result)
	case err := <-h.errc:
		return nil, err
	}
}

// HashPadded zero-pads any unwritten sections then computes the BMT root.
// Required for inclusion-proof generation so the tree is fully populated.
func (h *goroutineHasher) HashPadded(b []byte) ([]byte, error) {
	for i := h.size; i < h.maxSize; i += len(zerosection) {
		if _, err := h.Write(zerosection); err != nil {
			return nil, err
		}
	}
	return h.Hash(b)
}

// Reset prepares the Hasher for reuse.
func (h *goroutineHasher) Reset() {
	h.pos = 0
	h.size = 0
	copy(h.span, zerospan)
}

// Proof returns the inclusion proof of the i-th data segment.
func (h *goroutineHasher) Proof(i int) Proof {
	index := i
	if i < 0 || i > 127 {
		panic("segment index can only lie between 0-127")
	}
	i = i / 2
	n := h.bmt.leaves[i]
	isLeft := n.isLeft
	var sisters [][]byte
	for n = n.parent; n != nil; n = n.parent {
		sisters = append(sisters, n.getSister(isLeft))
		isLeft = n.isLeft
	}

	secsize := 2 * h.segmentSize
	offset := i * secsize
	section := make([]byte, secsize)
	copy(section, h.bmt.buffer[offset:offset+secsize])
	segment, firstSegmentSister := section[:h.segmentSize], section[h.segmentSize:]
	if index%2 != 0 {
		segment, firstSegmentSister = firstSegmentSister, segment
	}
	sisters = append([][]byte{firstSegmentSister}, sisters...)
	return Proof{segment, sisters, h.span, index}
}

// Verify reconstructs the BMT root from a proof for the i-th segment.
func (h *goroutineHasher) Verify(i int, proof Proof) (root []byte, err error) {
	var section []byte
	if i%2 == 0 {
		section = append(append(section, proof.ProveSegment...), proof.ProofSegments[0]...)
	} else {
		section = append(append(section, proof.ProofSegments[0]...), proof.ProveSegment...)
	}
	i = i / 2
	n := h.bmt.leaves[i]
	hasher := h.baseHasher()
	isLeft := n.isLeft
	root, err = doHash(hasher, section)
	if err != nil {
		return nil, err
	}
	n = n.parent

	for _, sister := range proof.ProofSegments[1:] {
		if isLeft {
			root, err = doHash(hasher, root, sister)
		} else {
			root, err = doHash(hasher, sister, root)
		}
		if err != nil {
			return nil, err
		}
		isLeft = n.isLeft
		n = n.parent
	}
	return doHash(hasher, proof.Span, root)
}

// processSection writes the hash of i-th section into level 1 node of the BMT tree.
func (h *goroutineHasher) processSection(i int, final bool) {
	secsize := 2 * h.segmentSize
	offset := i * secsize
	level := 1
	// select the leaf node for the section
	n := h.bmt.leaves[i]
	isLeft := n.isLeft
	hasher := n.hasher
	n = n.parent
	// hash the section
	section, err := doHash(hasher, h.bmt.buffer[offset:offset+secsize])
	if err != nil {
		select {
		case h.errc <- err:
		default:
		}
		return
	}
	// write hash into parent node
	if final {
		h.writeFinalNode(level, n, isLeft, section)
	} else {
		h.writeNode(n, isLeft, section)
	}
}

// writeNode pushes data up the tree. The second of two sibling writes hashes left|right
// and recurses to the parent; the first terminates. A single hasher is reused from that point
// up since the path is synchronous from there.
func (h *goroutineHasher) writeNode(n *goroutineNode, isLeft bool, s []byte) {
	var err error
	for {
		if n == nil {
			h.result <- s
			return
		}
		if isLeft {
			n.left = s
		} else {
			n.right = s
		}
		if n.toggle() {
			return
		}
		s, err = doHash(n.hasher, n.left, n.right)
		if err != nil {
			select {
			case h.errc <- err:
			default:
			}
			return
		}
		isLeft = n.isLeft
		n = n.parent
	}
}

// writeFinalNode follows the path from the final data segment to the BMT root.
// For unbalanced trees it fills the missing right-sister nodes from the pool's
// zerohashes lookup table. Otherwise behaves like writeNode.
func (h *goroutineHasher) writeFinalNode(level int, n *goroutineNode, isLeft bool, s []byte) {
	var err error
	for {
		if n == nil {
			if s != nil {
				h.result <- s
			}
			return
		}
		var noHash bool
		if isLeft {
			n.right = h.zerohashes[level]
			if s != nil {
				n.left = s
				noHash = false
			} else {
				noHash = n.toggle()
			}
		} else {
			if s != nil {
				n.right = s
				noHash = n.toggle()
			} else {
				noHash = true
			}
		}
		if noHash {
			s = nil
		} else {
			s, err = doHash(n.hasher, n.left, n.right)
			if err != nil {
				select {
				case h.errc <- err:
				default:
				}
				return
			}
		}
		isLeft = n.isLeft
		n = n.parent
		level++
	}
}
