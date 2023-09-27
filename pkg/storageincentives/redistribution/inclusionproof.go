// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redistribution

import (
	"encoding/binary"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storer"
	"github.com/ethersphere/bee/pkg/swarm"
)

type ChunkInclusionProofs struct {
	A ChunkInclusionProof `json:"proof1"`
	B ChunkInclusionProof `json:"proof2"`
	C ChunkInclusionProof `json:"proofLast"`
}

// ChunkInclusionProof structure must exactly match
// corresponding structure (of the same name) in Redistribution.sol smart contract.
// github.com/ethersphere/storage-incentives/blob/ph_f2/src/Redistribution.sol
// github.com/ethersphere/storage-incentives/blob/master/src/Redistribution.sol (when merged to master)
type ChunkInclusionProof struct {
	ProveSegment   common.Hash
	ProofSegments  []common.Hash
	ProofSegments2 []common.Hash
	ProveSegment2  common.Hash
	ChunkSpan      uint64
	ProofSegments3 []common.Hash
	PostageProof   PostageProof
	SocProof       []SOCProof
}

// SOCProof structure must exactly match
// corresponding structure (of the same name) in Redistribution.sol smart contract.
type PostageProof struct {
	Signature []byte
	PostageId []byte
	Index     uint64
	TimeStamp uint64
}

// SOCProof structure must exactly match
// corresponding structure (of the same name) in Redistribution.sol smart contract.
type SOCProof struct {
	Signer     []byte
	Signature  []byte
	Identifier common.Hash
	ChunkAddr  common.Hash
}

// Transforms arguments to ChunkInclusionProof object
func NewChunkInclusionProof(proofp1, proofp2 bmt.Proof, proofp3 bmt.Proof, sampleItem storer.SampleItem) (ChunkInclusionProof, error) {
	socProof, err := makeSOCProof(sampleItem)
	if err != nil {
		return ChunkInclusionProof{}, err
	}

	return ChunkInclusionProof{
		ProofSegments:  toCommonHash(proofp1.ProofSegments...),
		ProveSegment:   toCommonHash(proofp1.ProveSegment)[0],
		ProofSegments2: toCommonHash(proofp2.ProofSegments...),
		ProveSegment2:  toCommonHash(proofp2.ProveSegment)[0],
		ChunkSpan:      binary.LittleEndian.Uint64(proofp2.Span[:swarm.SpanSize]), // should be uint64 on the other size; copied from pkg/api/bytes.go
		ProofSegments3: toCommonHash(proofp3.ProofSegments...),
		PostageProof: PostageProof{
			Signature: sampleItem.Stamp.Sig(),
			PostageId: sampleItem.Stamp.BatchID(),
			Index:     binary.BigEndian.Uint64(sampleItem.Stamp.Index()),
			TimeStamp: binary.BigEndian.Uint64(sampleItem.Stamp.Timestamp()),
		},
		SocProof: socProof,
	}, nil
}

func toCommonHash(hashes ...[]byte) []common.Hash {
	output := make([]common.Hash, len(hashes))
	for i, s := range hashes {
		output[i] = common.BytesToHash(s)
	}
	return output
}

func makeSOCProof(sampleItem storer.SampleItem) ([]SOCProof, error) {
	var emptySOCProof = make([]SOCProof, 0)
	ch := swarm.NewChunk(sampleItem.ChunkAddress, sampleItem.ChunkData)
	if !soc.Valid(ch) {
		return emptySOCProof, nil
	}

	socCh, err := soc.FromChunk(ch)
	if err != nil {
		return emptySOCProof, err
	}

	return []SOCProof{{
		Signer:     socCh.OwnerAddress(),
		Signature:  socCh.Signature(),
		Identifier: common.BytesToHash(socCh.ID()),
		ChunkAddr:  common.BytesToHash(socCh.WrappedChunk().Address().Bytes()),
	}}, nil
}
