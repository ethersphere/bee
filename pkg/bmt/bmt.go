// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt

import (
	"encoding/binary"
	"hash"
)

var _ Hash = (*Hasher)(nil)

var zerospan = make([]byte, 8)

// Hasher a reusable hasher for fixed maximum size chunks representing a BMT
// It reuses a pool of trees for amortised memory allocation and resource control,
// and supports order-agnostic concurrent segment writes and section (double segment) writes
// as well as sequential read and write.
//
// The same hasher instance must not be called concurrently on more than one chunk.
//
// The same hasher instance is synchronously reuseable.
//
// Sum gives back the tree to the pool and guaranteed to leave
// the tree and itself in a state reusable for hashing a new chunk.
type Hasher struct {
	*Conf              // configuration
	bmt    *tree       // prebuilt BMT resource for flowcontrol and proofs
	size   int         // bytes written to Hasher since last Reset()
	pos    int         // index of rightmost currently open segment
	offset int         // offset (cursor position) within currently open segment
	result chan []byte // result channel
	span   []byte      // The span of the data subsumed under the chunk
}

// Capacity returns the maximum amount of bytes that will be processed by this hasher implementation.
func (h *Hasher) Capacity() int {
	return h.maxSize
}

// writeSection allows asynchronous writes of the base segments
func (h *Hasher) WriteSection(idx int, data []byte) {
	// secsize := 2 * h.segmentCount
	// l := len(data)
	// if secsize < l {
	// 	l = secsize
	// }
	// copy(h.bmt.buffer[idx*secsize:], data[:l])
	// if h.pos > idx {
	// 	go h.processSection(idx, false)
	// } else {
	// 	h.pos = idx
	// }
}

// SetSpan sets the span length value prefix in numeric form for the current hash operation.
func (h *Hasher) SetSpan(length int64) {
	binary.LittleEndian.PutUint64(h.span, uint64(length))
}

// SetSpanBytes sets the span length value prefix in bytes for the current hash operation.
func (h *Hasher) SetSpanBytes(span []byte) {
	copy(h.span, span)
}

// Size returns the digest size of the hash
func (h *Hasher) Size() int {
	return h.segmentSize
}

// BlockSize returns the optimal write size to the Hasher
func (h *Hasher) BlockSize() int {
	return 2 * h.segmentSize
}

// Sum returns the BMT root hash of the buffer
// using Sum presupposes sequential synchronous writes (io.Writer interface).
func (h *Hasher) Sum(b []byte) []byte {
	if h.size == 0 && h.offset == 0 {
		return h.GetZeroHash()
	}

	zeros := make([]byte, 2*h.segmentSize)
	copy(h.bmt.buffer[h.size:], zeros)
	// write the last section with final flag set to true
	go h.processSection(h.pos, true)
	return doSum(h.hasher(), b, h.span, <-h.result)
}

// Write calls sequentially add to the buffer to be hashed,
// with every full segment calls processSection in a go routine.
func (h *Hasher) Write(b []byte) (int, error) {
	copy(h.bmt.buffer[h.size:], b)
	l := len(b)
	max := h.maxSize - h.size
	if l > max {
		l = max
	}
	secsize := 2 * h.segmentSize
	from := h.size / secsize
	h.offset = h.size % secsize
	h.size += l
	to := h.size / secsize
	if l == max {
		to--
	}
	h.pos = to
	for i := from; i < to; i++ {
		go h.processSection(i, false)
	}
	return l, nil
}

// Reset prepares the Hasher for reuse
func (h *Hasher) Reset() {
	h.pos = 0
	h.size = 0
	h.offset = 0
	copy(h.span, zerospan)
}

// LengthToSpan creates a binary data span size representation.
// It is required for calculating the BMT hash.
func LengthToSpan(span []byte, length int64) {
	binary.LittleEndian.PutUint64(span, uint64(length))
}

// GetZeroHash returns the zero hash of the full depth of the Hasher instance.
func (h *Hasher) GetZeroHash() []byte {
	return h.zerohashes[h.depth]
}

// processSection writes the hash of i-th section into level 1 node of the BMT tree.
func (h *Hasher) processSection(i int, final bool) {
	// select the leaf node for the section
	secsize := 2 * h.segmentSize
	offset := i * secsize
	section := h.bmt.buffer[offset : offset+secsize]
	level := 1
	n := h.bmt.leaves[i]
	isLeft := n.isLeft
	hasher := n.hasher
	n = n.parent
	// hash the section
	section = doSum(hasher, nil, section)
	// write hash into parent node
	if final {
		// for the last segment use writeFinalNode
		h.writeFinalNode(level, n, isLeft, section)
	} else {
		h.writeNode(n, isLeft, section)
	}
}

// calculates the hash of the data using hash.Hash.
//
// BUG: This legacy implementation has no error handling for the writer. Use with caution.
func doSum(h hash.Hash, b []byte, data ...[]byte) []byte {
	h.Reset()
	for _, v := range data {
		_, _ = h.Write(v)
	}
	return h.Sum(b)
}

// writeNode pushes the data to the node.
// if it is the first of 2 sisters written, the routine terminates.
// if it is the second, it calculates the hash and writes it
// to the parent node recursively.
// since hashing the parent is synchronous the same hasher can be used.
func (h *Hasher) writeNode(n *node, isLeft bool, s []byte) {
	level := 1
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
		s = doSum(n.hasher, nil, n.left, n.right)
		isLeft = n.isLeft
		n = n.parent
		level++
	}
}

// writeFinalNode is following the path starting from the final datasegment to the
// BMT root via parents.
// For unbalanced trees it fills in the missing right sister nodes using
// the pool's lookup table for BMT subtree root hashes for all-zero sections.
// Otherwise behaves like `writeNode`.
func (h *Hasher) writeFinalNode(level int, n *node, isLeft bool, s []byte) {
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
			s = doSum(n.hasher, nil, n.left, n.right)
		}
		// iterate to parent
		isLeft = n.isLeft
		n = n.parent
		level++
	}
}
