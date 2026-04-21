// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt

// Prover adds Merkle-proof generation and verification on top of a BMT hasher.
//
// Proof generation requires the tree to be fully populated, so Prover.Hash
// zero-pads any unwritten sections before hashing. Prover is always backed by
// the goroutine BMT implementation, independent of the SIMD opt-in flag:
// proofs are produced on a rare, redistribution-only code path where the
// well-tested goroutine implementation is preferred over the SIMD speedup.
//
// The embedded *goroutineHasher is package-private, so callers outside
// pkg/bmt cannot bypass Hash/Sum to skip padding.
type Prover struct {
	*goroutineHasher
}

// Proof represents a Merkle proof of segment.
type Proof struct {
	ProveSegment  []byte
	ProofSegments [][]byte
	Span          []byte
	Index         int
}

// Hash zero-pads any unwritten sections (so every leaf section in the BMT is
// populated and Proof paths are reconstructible), then computes the BMT root.
// Shadows the promoted goroutineHasher.Hash.
func (p *Prover) Hash(b []byte) ([]byte, error) {
	for i := p.size; i < p.maxSize; i += len(zerosection) {
		if _, err := p.Write(zerosection); err != nil {
			return nil, err
		}
	}
	return p.goroutineHasher.Hash(b)
}

// Sum shadows the promoted goroutineHasher.Sum so Prover used as a hash.Hash
// also pads. Without this override Sum would call the unpadded Hash and
// produce a different digest than Prover.Hash.
func (p *Prover) Sum(b []byte) []byte {
	s, _ := p.Hash(b)
	return s
}

// NewProver returns a Prover backed by a freshly allocated goroutine-based
// BMT hasher, independent of the SIMD opt-in flag.
func NewProver() *Prover {
	return &Prover{goroutineHasher: newGoroutineHasher()}
}

// NewPrefixProver is NewProver with an optional keccak prefix prepended to every
// BMT node hash. Also goroutine-backed regardless of SIMDOptIn.
func NewPrefixProver(prefix []byte) *Prover {
	return &Prover{goroutineHasher: newGoroutinePrefixHasher(prefix)}
}

// ProverPool is a pool of goroutine-backed Provers. Ignores SIMDOptIn by design.
type ProverPool interface {
	GetProver() *Prover
	PutProver(*Prover)
}

// NewProverPool returns a pool of goroutine-backed Provers, independent of
// SIMDOptIn. See Prover for the rationale.
func NewProverPool(c *Conf) ProverPool {
	return newGoroutineProverPool(c)
}
