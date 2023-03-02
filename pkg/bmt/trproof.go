// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt

// Prover wraps the Hasher to allow Merkle proof functionality
type TrProver struct {
	*TrHasher
}

func NewTrProover(key []byte) *TrProver {
	return &TrProver{
		TrHasher: NewTrHasher(key),
	}
}

// Proof returns the inclusion proof of the i-th data segment
func (p TrProver) Proof(i int) Proof {
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
	return Proof{section, sisters, p.span}
}

// Verify returns the bmt hash obtained from the proof which can then be checked against
// the BMT hash of the chunk
func (p TrProver) Verify(i int, key []byte, proof Proof) (root []byte, err error) {
	i = i / 2
	n := p.bmt.leaves[i]
	isLeft := n.isLeft
	root, err = trDoHash(n.hasher, key, proof.ProveSegment)
	if err != nil {
		return nil, err
	}
	n = n.parent

	for _, sister := range proof.ProofSegments {
		if isLeft {
			root, err = trDoHash(n.hasher, key, root, sister)
		} else {
			root, err = trDoHash(n.hasher, key, sister, root)
		}
		if err != nil {
			return nil, err
		}
		isLeft = n.isLeft
		n = n.parent
	}
	return keyedHash(key, proof.Span, root)
}
