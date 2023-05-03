// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redistribution

import (
	"testing"

	"github.com/ethersphere/bee/pkg/storageincentives/types"
)

type ChunkInclusionProofs = types.Trio[ChunkInclusionProof]

type ChunkInclusionProof struct {
	ProofSegments  [][]byte
	ProveSegment   []byte
	ProofSegments2 [][]byte
	ProveSegment2  []byte
	ChunkSpan      uint64
	ProofSegments3 [][]byte

	Signer    []byte
	Signature []byte
	ChunkAddr []byte
	PostageId []byte
	Index     []byte
	TimeStamp []byte
}

func RandChunkInclusionProof(t *testing.T) ChunkInclusionProof {
	t.Helper()

	return ChunkInclusionProof{}
}

func RandChunkInclusionProofs(t *testing.T) ChunkInclusionProofs {
	t.Helper()

	return ChunkInclusionProofs{
		Element1: RandChunkInclusionProof(t),
		Element2: RandChunkInclusionProof(t),
		Element3: RandChunkInclusionProof(t),
	}
}
