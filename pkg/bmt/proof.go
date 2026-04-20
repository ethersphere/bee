// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt

// Prover wraps a BMT Hasher to add Merkle-proof convenience methods.
//
// Proof generation requires the tree to be fully populated, so Prover.Hash
// zero-pads any unwritten sections before hashing (unlike the bare Hash method
// on the underlying hasher). Proof and Verify are promoted from the embedded
// Hasher.
type Prover struct {
	Hasher
}

// Proof represents a Merkle proof of segment
type Proof struct {
	ProveSegment  []byte
	ProofSegments [][]byte
	Span          []byte
	Index         int
}

// Hash computes the BMT root with zero-padding, so the proof tree is fully populated.
// It shadows the embedded Hasher.Hash to preserve the existing Prover API.
func (p Prover) Hash(b []byte) ([]byte, error) {
	return p.Hasher.HashPadded(b)
}
