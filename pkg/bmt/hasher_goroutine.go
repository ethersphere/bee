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

// SetHeaderInt64 sets the span preamble from an int64.
func (h *goroutineHasher) SetHeaderInt64(length int64) {
	binary.LittleEndian.PutUint64(h.span, uint64(length))
}

// SetHeader copies the span preamble from the argument.
func (h *goroutineHasher) SetHeader(span []byte) { copy(h.span, span) }

// Write appends to the chunk buffer; each complete section triggers a goroutine-level hash.
func (h *goroutineHasher) Write(b []byte) (int, error) {
	// clamp the input to whatever capacity is left in the buffer; extra bytes are silently dropped.
	l := len(b)
	maxVal := h.maxSize - h.size
	if l > maxVal {
		l = maxVal
	}
	// copy the new bytes into the leaf-level buffer at the current write cursor.
	copy(h.bmt.buffer[h.size:], b)
	// translate byte offsets to section indices: `from` is the first section newly touched,
	// `to` is the first section *not* yet fully populated by this write.
	secsize := 2 * h.segmentSize
	from := h.size / secsize
	h.size += l
	to := h.size / secsize
	// when this Write fills the buffer exactly, hold back the last section so the
	// final Sum call is the one that hashes it (with the writeFinalNode path).
	if l == maxVal {
		to--
	}
	// remember where the final section lives so Sum can kick it off.
	h.pos = to
	// fan out one goroutine per fully-populated section to start hashing it concurrently.
	for i := from; i < to; i++ {
		go h.processSection(i, false)
	}
	return l, nil
}

// Sum returns the BMT root hash of the buffer written so far appended to b,
// satisfying the standard library hash.Hash interface.
func (h *goroutineHasher) Sum(b []byte) []byte {
	// nothing was written: the BMT root is the all-zero subtree hash at depth h.depth.
	// Still wrap with the span so the output shape matches a normal chunk hash.
	if h.size == 0 {
		out, _ := doHash(h.baseHasher(), h.span, h.zerohashes[h.depth])
		return append(b, out...)
	}
	// zero-pad the trailing partial section so the final-section hasher sees a
	// deterministic 64-byte input regardless of how Write was sliced.
	copy(h.bmt.buffer[h.size:], zerosection)
	// hash the last section on its own goroutine with the final flag set, which
	// fills missing right-sister branches with all-zero subtree hashes on its way up.
	go h.processSection(h.pos, true)
	// wait for either the BMT root to bubble up via h.result or an error from one of
	// the per-section goroutines via h.errc; the error is swallowed because the
	// hash.Hash.Sum contract has no error return.
	var inner []byte
	select {
	case result := <-h.result:
		inner = result
	case <-h.errc:
	}
	// wrap the BMT root with the span (and any configured prefix via baseHasher)
	// to produce the final chunk address, then append it to b.
	out, _ := doHash(h.baseHasher(), h.span, inner)
	return append(b, out...)
}

// Reset prepares the Hasher for reuse.
func (h *goroutineHasher) Reset() {
	h.pos = 0
	h.size = 0
	copy(h.span, zerospan)
}

// Proof returns the inclusion proof of the i-th data segment.
func (h *goroutineHasher) Proof(i int) Proof {
	// preserve the original 32-byte segment index — needed to know whether the
	// proven segment is the left or right half of its leaf section.
	index := i
	if i < 0 || i > 127 {
		panic("segment index can only lie between 0-127")
	}
	// each leaf section holds two 32-byte segments; map the segment index to its leaf index.
	i = i / 2
	// walk from the leaf up to the root, collecting each level's sister hash in order.
	// after the SIMD/goroutine pass these sisters are already cached on the parent nodes.
	n := h.bmt.leaves[i]
	isLeft := n.isLeft
	var sisters [][]byte
	for n = n.parent; n != nil; n = n.parent {
		sisters = append(sisters, n.getSister(isLeft))
		isLeft = n.isLeft
	}

	// re-read the proven section so we can split it into (segment, firstSegmentSister);
	// we copy because callers must be free to use the proof after Reset overwrites the buffer.
	secsize := 2 * h.segmentSize
	offset := i * secsize
	section := make([]byte, secsize)
	copy(section, h.bmt.buffer[offset:offset+secsize])
	segment, firstSegmentSister := section[:h.segmentSize], section[h.segmentSize:]
	// odd segment index means the proven segment is the right half — swap so `segment`
	// is always the proven one regardless of left/right position.
	if index%2 != 0 {
		segment, firstSegmentSister = firstSegmentSister, segment
	}
	// the proof lists sisters bottom-up; prepend the leaf-level sister so position 0
	// is always the immediate sibling of the proven segment.
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
		// for the last segment use writeFinalNode
		h.writeFinalNode(level, n, isLeft, section)
	} else {
		h.writeNode(n, isLeft, section)
	}
}

// writeNode pushes the data to the node.
// if it is the first of 2 sisters written, the routine terminates.
// if it is the second, it calculates the hash and writes it
// to the parent node recursively.
// since hashing the parent is synchronous the same hasher can be used.
func (h *goroutineHasher) writeNode(n *goroutineNode, isLeft bool, s []byte) {
	var err error
	for {
		// at the root of the bmt just write the result to the result channel
		if n == nil {
			h.result <- s
			return
		}
		// otherwise assign child hash to left or right segment
		if isLeft {
			n.left = s
		} else {
			n.right = s
		}
		// the child-thread first arriving will terminate
		if n.toggle() {
			return
		}
		// the thread coming second now can be sure both left and right children are written
		// so it calculates the hash of left|right and pushes it to the parent
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

// writeFinalNode is following the path starting from the final datasegment to the
// BMT root via parents.
// For unbalanced trees it fills in the missing right sister nodes using
// the pool's lookup table for BMT subtree root hashes for all-zero sections.
// Otherwise behaves like `writeNode`.
func (h *goroutineHasher) writeFinalNode(level int, n *goroutineNode, isLeft bool, s []byte) {
	var err error
	for {
		// at the root of the bmt just write the result to the result channel
		if n == nil {
			if s != nil {
				h.result <- s
			}
			return
		}
		var noHash bool
		if isLeft {
			// coming from left sister branch
			// when the final section's path is going via left child node
			// we include an all-zero subtree hash for the right level and toggle the node.
			n.right = h.zerohashes[level]
			if s != nil {
				n.left = s
				// if a left final node carries a hash, it must be the first (and only thread)
				// so the toggle is already in passive state no need no call
				// yet thread needs to carry on pushing hash to parent
				noHash = false
			} else {
				// if again first thread then propagate nil and calculate no hash
				noHash = n.toggle()
			}
		} else {
			// right sister branch
			if s != nil {
				// if hash was pushed from right child node, write right segment change state
				n.right = s
				// if toggle is true, we arrived first so no hashing just push nil to parent
				noHash = n.toggle()
			} else {
				// if s is nil, then thread arrived first at previous node and here there will be two,
				// so no need to do anything and keep s = nil for parent
				noHash = true
			}
		}
		// the child-thread first arriving will just continue resetting s to nil
		// the second thread now can be sure both left and right children are written
		// it calculates the hash of left|right and pushes it to the parent
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
		// iterate to parent
		isLeft = n.isLeft
		n = n.parent
		level++
	}
}
