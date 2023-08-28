// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redistribution

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/storageincentives/types"
	"github.com/ethersphere/bee/pkg/util/testutil"
)

type ChunkInclusionProofs = types.Trio[ChunkInclusionProof]

// ChunkInclusionProof structure must exactly match
// corresponding structure (of the same name) in Redistribution.sol smart contract.
// github.com/ethersphere/storage-incentives/blob/ph_f2/src/Redistribution.sol
// github.com/ethersphere/storage-incentives/blob/master/src/Redistribution.sol (when merged to master)
type ChunkInclusionProof struct {
	ProofSegments  []string
	ProveSegment   string
	ProofSegments2 []string
	ProveSegment2  string
	ChunkSpan      uint64
	ProofSegments3 []string

	Signature string
	ChunkAddr string
	PostageId string
	Index     uint64
	TimeStamp uint64

	SocProofAttached []SOCProof
}

// SOCProof structure must exactly match
// corresponding structure (of the same name) in Redistribution.sol smart contract.
type SOCProof struct {
	Signer     common.Address
	Signature  string
	Identifier string
	ChunkAddr  string
}

func RandChunkInclusionProof(t *testing.T) ChunkInclusionProof {
	t.Helper()

	return ChunkInclusionProof{
		ProofSegments:  []string{types.ToHexString(testutil.RandBytes(t, 32))},
		ProveSegment:   types.ToHexString(testutil.RandBytes(t, 32)),
		ProofSegments2: []string{types.ToHexString(testutil.RandBytes(t, 32))},
		ProveSegment2:  types.ToHexString(testutil.RandBytes(t, 32)),
		ProofSegments3: []string{types.ToHexString(testutil.RandBytes(t, 32))},
		ChunkSpan:      1,
		Signature:      string(testutil.RandBytes(t, 32)),
		ChunkAddr:      types.ToHexString(testutil.RandBytes(t, 32)),
		PostageId:      types.ToHexString(testutil.RandBytes(t, 32)),
		Index:          1,
		TimeStamp:      uint64(time.Now().Unix()),
	}
}

func RandChunkInclusionProofs(t *testing.T) ChunkInclusionProofs {
	t.Helper()

	return ChunkInclusionProofs{
		A: RandChunkInclusionProof(t),
		B: RandChunkInclusionProof(t),
		C: RandChunkInclusionProof(t),
	}
}
