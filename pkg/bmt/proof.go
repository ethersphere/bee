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

// Hash overrides base hash function of Hasher to fill buffer with zeros until chunk length
func (p Prover) Hash(b []byte) ([]byte, error) {
	for i := p.size; i < p.maxSize; i += len(zerosection) {
		_, err := p.Hasher.Write(zerosection)
		if err != nil {
			return nil, err
		}
	}
	return p.Hasher.Hash(b)
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
	section := make([]byte, secsize)
	copy(section, p.bmt.buffer[offset:offset+secsize])
	segment, firstSegmentSister := section[:p.segmentSize], section[p.segmentSize:]
	if index%2 != 0 {
		segment, firstSegmentSister = firstSegmentSister, segment
	}
	sisters = append([][]byte{firstSegmentSister}, sisters...)
	return Proof{segment, sisters, p.span, index}
}

// Verify returns the bmt hash obtained from the proof which can then be checked against
// the BMT hash of the chunk
func (p Prover) Verify(i int, proof Proof) (root []byte, err error) {
	var section []byte
	if i%2 == 0 {
		section = append(append(section, proof.ProveSegment...), proof.ProofSegments[0]...)
	} else {
		section = append(append(section, proof.ProofSegments[0]...), proof.ProveSegment...)
	}
	i = i / 2
	n := p.bmt.leaves[i]
	hasher := p.hasher()
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
