// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt

// Prover wraps the Hasher to allow Merkle proof functionality
type Prover struct {
	*Hasher
}

// Proof represents a Merkle proof of segment
type Proof struct {
	ProveSegment  []byte
	ProofSegments [][]byte
	Span          []byte
	Index         int
}

// Proof returns the inclusion proof of the i-th data segment
func (p Prover) Proof(i int) Proof {
	index := i

	if i < 0 || i > 127 {
		panic("segment index can only lie between 0-127")
	}
	i = i / 2
	n := p.bmt.leaves[i]
	isLeft := n.isLeft
	var sisters [][]byte
	for n = n.parent; n != nil; n = n.parent {
		sisters = append(sisters, n.getSister(isLeft))
		isLeft = n.isLeft
	}

	secsize := 2 * p.segmentSize
	offset := i * secsize
	section := p.bmt.buffer[offset : offset+secsize]
	return Proof{section, sisters, p.span, index}
}

// Verify returns the bmt hash obtained from the proof which can then be checked against
// the BMT hash of the chunk
func (p Prover) Verify(i int, proof Proof) (root []byte, err error) {
	i = i / 2
	n := p.bmt.leaves[i]
	isLeft := n.isLeft
	root, err = doHash(n.hasher, proof.ProveSegment)
	if err != nil {
		return nil, err
	}
	n = n.parent

	for _, sister := range proof.ProofSegments {
		if isLeft {
			root, err = doHash(n.hasher, root, sister)
		} else {
			root, err = doHash(n.hasher, sister, root)
		}
		if err != nil {
			return nil, err
		}
		isLeft = n.isLeft
		n = n.parent
	}
	return doHash(n.hasher, proof.Span, root)
}

func (n *node) getSister(isLeft bool) []byte {
	var src []byte

	if isLeft {
		src = n.right
	} else {
		src = n.left
	}

	buf := make([]byte, len(src))
	copy(buf, src)
	return buf
}
