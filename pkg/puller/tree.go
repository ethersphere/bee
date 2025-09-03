// Copyright 2025 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package puller

import (
	"bytes"
	"errors"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var errShouldNeverHappen = errors.New("should never happen in case of keys have the same length and method was called from root")

// TreeNode is a leaf compacted binary tree
// representing the address space of the neighborhood
type TreeNode[T any] struct {
	K []byte
	V *T
	L uint8
	C [2]*TreeNode[T]
}

// Put will override the value if the key is already present
func (t *TreeNode[T]) Put(key []byte, p *T) *TreeNode[T] {
	bitIndex, err := bitOfBytes(key, t.L)
	if err != nil {
		// cannot go further in the binary representation, override leaf
		if t.V != nil && !bytes.Equal(t.K, key) {
			panic(errShouldNeverHappen)
		}
		t.V = p
		t.K = key
		return t
	}

	c := t.C[bitIndex]
	if c != nil {
		return c.Put(key, p)
	}

	if t.C[1-bitIndex] == nil {
		// both children are nil, we are on a leaf.
		if t.V == nil || bytes.Equal(t.K, key) {
			t.V = p
			t.K = key
			return t
		}

		// create as many parent tree nodes as needed
		po := swarm.Proximity(t.K, key)
		parent := t
		ci := bitIndex
		for i := uint8(0); i < po-t.L; i++ {
			parent.C[ci] = newTreeNode[T](nil, nil, parent.L+1)
			parent = parent.C[ci]
			ci, err = bitOfBytes(key, parent.L)
			if err != nil {
				panic(errShouldNeverHappen)
			}
		}

		// move the old leaf value to the new parent
		parent.C[1-ci] = newTreeNode(t.K, t.V, parent.L+1)
		t.V = nil
		t.K = nil

		// insert p to the new parent
		parent.C[ci] = newTreeNode(key, p, parent.L+1)
		return parent.C[ci]
	}

	// child slot is free on the node so peer can be inserted.
	c = newTreeNode(key, p, t.L+1)
	t.C[bitIndex] = c
	return c
}

// traverse assigns bins to each peer for syncing by traversing the tree
// func (t *TreeNode[T]) traverse() {
// 	// TODO
// }

func newTreeNode[T any](key []byte, p *T, level uint8) *TreeNode[T] {
	return &TreeNode[T]{
		K: key,
		V: p,
		L: level,
		C: [2]*TreeNode[T]{},
	}
}

// bitOfBytes extracts the bit at the specified index from a byte slice.
// Returns 0 or 1 based on the bit value at the given position.
func bitOfBytes(bytes []byte, bitIndex uint8) (uint8, error) {
	if bitIndex >= uint8(len(bytes)*8) {
		return 0, errors.New("bit index out of range")
	}
	byteIndex := bitIndex / 8
	bitPosition := 7 - (bitIndex % 8) // MSB first (big-endian)

	b := bytes[byteIndex]
	return (b >> bitPosition) & 1, nil
}
