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
	ProofSegments  []string `json:"proofSegments"`
	ProveSegment   string   `json:"proveSegment"`
	ProofSegments2 []string `json:"proofSegments2"`
	ProveSegment2  string   `json:"proveSegment2"`
	ChunkSpan      uint64   `json:"chunkSpan"`
	ProofSegments3 []string `json:"proofSegments3"`

	Signature string `json:"signature"`
	ChunkAddr string `json:"chunkAddr"`
	PostageId string `json:"postageId"`
	Index     string `json:"index"`
	TimeStamp uint64 `json:"timeStamp"`

	SocProofAttached []SOCProof `json:"socProofAttached"`
}

// SOCProof structure must exactly match
// corresponding structure (of the same name) in Redistribution.sol smart contract.
type SOCProof struct {
	Signer     common.Address `json:"signer"`
	Signature  string         `json:"signature"`
	Identifier string         `json:"identifier"`
	ChunkAddr  string         `json:"chunkAddr"`
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
		Index:          types.ToHexString(testutil.RandBytes(t, 32)),
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
