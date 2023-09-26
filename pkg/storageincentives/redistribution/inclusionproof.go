// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package redistribution

import (
	"encoding/binary"

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
	ProveSegment   []byte
	ProofSegments  [][]byte
	ProofSegments2 [][]byte
	ProveSegment2  []byte
	ChunkSpan      uint64
	ProofSegments3 [][]byte
	PostageProof   PostageProof
	SocProof       []SOCProof
}

// SOCProof structure must exactly match
// corresponding structure (of the same name) in Redistribution.sol smart contract.
type PostageProof struct {
	Signature []byte
	PostageId []byte
	Index     []byte
	TimeStamp []byte
}

// SOCProof structure must exactly match
// corresponding structure (of the same name) in Redistribution.sol smart contract.
type SOCProof struct {
	Signer     []byte
	Signature  []byte
	Identifier []byte
	ChunkAddr  []byte
}

// Transforms arguments to ChunkInclusionProof object
func NewChunkInclusionProof(proofp1, proofp2 bmt.Proof, proofp3 bmt.Proof, sampleItem storer.SampleItem) (ChunkInclusionProof, error) {
	socProof, err := makeSOCProof(sampleItem)
	if err != nil {
		return ChunkInclusionProof{}, err
	}

	return ChunkInclusionProof{
		ProofSegments:  proofp1.ProofSegments,
		ProveSegment:   proofp1.ProveSegment,
		ProofSegments2: proofp2.ProofSegments,
		ProveSegment2:  proofp2.ProveSegment,
		ChunkSpan:      binary.LittleEndian.Uint64(proofp2.Span[:swarm.SpanSize]), // should be uint64 on the other size; copied from pkg/api/bytes.go
		ProofSegments3: proofp3.ProofSegments,
		PostageProof: PostageProof{
			Signature: sampleItem.Stamp.Sig(),
			PostageId: sampleItem.Stamp.BatchID(),
			Index:     sampleItem.Stamp.Index(),
			TimeStamp: sampleItem.Stamp.Timestamp(),
		},
		SocProof: socProof,
	}, nil
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
		Identifier: socCh.ID(),
		ChunkAddr:  socCh.WrappedChunk().Address().Bytes(),
	}}, nil
}
