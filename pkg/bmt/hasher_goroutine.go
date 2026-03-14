// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux || !amd64 || purego

package bmt

import (
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// Hasher is a reusable hasher for fixed maximum size chunks representing a BMT.
// This implementation uses goroutine-based concurrent tree traversal.
//
// It reuses a pool of trees for amortised memory allocation and resource control,
// and supports order-agnostic concurrent segment writes and section (double segment) writes
// as well as sequential read and write.
//
// The same hasher instance must not be called concurrently on more than one chunk.
//
// The same hasher instance is synchronously reusable.
//
// Sum gives back the tree to the pool and guaranteed to leave
// the tree and itself in a state reusable for hashing a new chunk.
type Hasher struct {
	*Conf              // configuration
	bmt    *tree       // prebuilt BMT resource for flowcontrol and proofs
	size   int         // bytes written to Hasher since last Reset()
	pos    int         // index of rightmost currently open segment
	result chan []byte // result channel
	errc   chan error  // error channel
	span   []byte      // The span of the data subsumed under the chunk
}

// NewHasher gives back an instance of a Hasher struct
func NewHasher() *Hasher {
	return newHasherWithConf(NewConf(swarm.BmtBranches, 32))
}

// NewPrefixHasher gives back an instance of a Hasher struct with the given prefix
// prepended to every hash operation.
func NewPrefixHasher(prefix []byte) *Hasher {
	return newHasherWithConf(NewConfWithPrefix(prefix, swarm.BmtBranches, 32))
}

func newHasherWithConf(conf *Conf) *Hasher {
	return &Hasher{
		Conf:   conf,
		result: make(chan []byte),
		errc:   make(chan error, 1),
		span:   make([]byte, SpanSize),
		bmt:    newTree(conf.maxSize, conf.depth, conf.hasherFunc),
	}
}

// Write calls sequentially add to the buffer to be hashed,
// with every full segment calls processSection in a go routine.
func (h *Hasher) Write(b []byte) (int, error) {
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

// Hash returns the BMT root hash of the buffer and an error
// using Hash presupposes sequential synchronous writes (io.Writer interface).
func (h *Hasher) Hash(b []byte) ([]byte, error) {
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

// Reset prepares the Hasher for reuse
func (h *Hasher) Reset() {
	h.pos = 0
	h.size = 0
	copy(h.span, zerospan)
}

// processSection writes the hash of i-th section into level 1 node of the BMT tree.
func (h *Hasher) processSection(i int, final bool) {
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
func (h *Hasher) writeNode(n *node, isLeft bool, s []byte) {
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
func (h *Hasher) writeFinalNode(level int, n *node, isLeft bool, s []byte) {
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
