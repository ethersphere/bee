// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bmt implements Binary Merkle Tree hash.
// Binary Merkle Tree Hash is a hash function over arbitrary datachunks of limited size.
// The BMT hash is defined as H(span|bmt-root) where span is an 8-byte metadata prefix and
// bmt-root is the root hash of the binary merkle tree built over fixed size segments
// of the underlying chunk using any base hash function H (e.g., keccak 256 SHA3).
// The number of segments on the base must be a power of 2 so that the resulting tree is balanced.
// Chunks with data shorter than the fixed size are hashed as if they had zero padding.
//
// BMT hash is used as the chunk hash function in swarm which in turn is the basis for the
// 128 branching swarm hash http://swarm-guide.readthedocs.io/en/latest/architecture.html#swarm-hash
//
// The BMT is optimal for providing compact inclusion proofs, i.e. prove that a
// segment is a substring of a chunk starting at a particular offset.
// The size of the underlying segments is fixed to the size of the base hash (called the resolution
// of the BMT hash), Using Keccak256 SHA3 hash is 32 bytes, the EVM word size to optimize for on-chain BMT verification
// as well as the hash size optimal for inclusion proofs in the merkle tree of the swarm hash.
//
// Two implementations are provided:
//
// RefHasher is optimized for code simplicity and meant as a reference implementation
// that is simple to understand
//
// Hasher is optimized for speed taking advantage of concurrency with minimalistic
// control structure to coordinate the concurrent routines
//
// BMT Hasher implements the following interfaces:
//
// standard golang hash.Hash - synchronous, reusable
//
// io.Writer - synchronous left-to-right datawriter
package bmt
