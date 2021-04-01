// Copyright 2018 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Binary Merkle Tree Hash is a hash function over arbitrary datachunks of limited size.
// It is defined as the root hash of the binary merkle tree built over fixed size segments
// of the underlying chunk using any base hash function (e.g., keccak 256 SHA3).
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
package legacy
