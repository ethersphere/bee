// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redistribution

import (
	"testing"

	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/storageincentives/types"
	"github.com/ethersphere/bee/pkg/swarm"
)

type ChunkInclusionProofs = types.Trio[ChunkInclusionProof]

type ChunkInclusionProof struct {
	PostageStamp            swarm.Stamp
	WitnessProof            bmt.Proof
	RetentionProof          bmt.Proof
	TransformedAddressProof bmt.Proof
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
