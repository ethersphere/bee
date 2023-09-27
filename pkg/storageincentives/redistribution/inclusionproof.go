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
	ProofSegments  [7]common.Hash
	ProofSegments2 [7]common.Hash
	ProveSegment2  common.Hash
	ChunkSpan      uint64
	ProofSegments3 [7]common.Hash
	PostageProof   PostageProof
	SocProof       [1]SOCProof
}

// SOCProof structure must exactly match
// corresponding structure (of the same name) in Redistribution.sol smart contract.
type PostageProof struct {
	Signature [65]byte
	PostageId common.Hash
	Index     uint64
	TimeStamp uint64
}

// SOCProof structure must exactly match
// corresponding structure (of the same name) in Redistribution.sol smart contract.
type SOCProof struct {
	Signer     common.Address
	Signature  [65]byte
	Identifier common.Hash
	ChunkAddr  common.Hash
}

// Transforms arguments to ChunkInclusionProof object
func NewChunkInclusionProof(proofp1, proofp2 bmt.Proof, proofp3 bmt.Proof, sampleItem storer.SampleItem) (ChunkInclusionProof, error) {
	socProof, err := makeSOCProof(sampleItem)
	if err != nil {
		return ChunkInclusionProof{}, err
	}
	var signature [65]byte
	copy(signature[:], sampleItem.Stamp.Sig())
	var batchId [32]byte
	copy(batchId[:], sampleItem.Stamp.BatchID())

	return ChunkInclusionProof{
		ProofSegments:  toCommonHash(proofp1.ProofSegments),
		ProveSegment:   common.BytesToHash(proofp1.ProveSegment),
		ProofSegments2: toCommonHash(proofp2.ProofSegments),
		ProveSegment2:  common.BytesToHash(proofp2.ProveSegment),
		ChunkSpan:      binary.LittleEndian.Uint64(proofp2.Span[:swarm.SpanSize]), // should be uint64 on the other size; copied from pkg/api/bytes.go
		ProofSegments3: toCommonHash(proofp3.ProofSegments),
		PostageProof: PostageProof{
			Signature: signature,
			PostageId: batchId,
			Index:     binary.BigEndian.Uint64(sampleItem.Stamp.Index()),
			TimeStamp: binary.BigEndian.Uint64(sampleItem.Stamp.Timestamp()),
		},
		SocProof: socProof,
	}, nil
}

func toCommonHash(hashes [][]byte) [7]common.Hash {
	var output [7]common.Hash
	for i, s := range hashes {
		output[i] = common.BytesToHash(s)
	}
	return output
}

func makeSOCProof(sampleItem storer.SampleItem) ([1]SOCProof, error) {
	var emptySOCProof [1]SOCProof
	ch := swarm.NewChunk(sampleItem.ChunkAddress, sampleItem.ChunkData)
	if !soc.Valid(ch) {
		return emptySOCProof, nil
	}

	socCh, err := soc.FromChunk(ch)
	if err != nil {
		return emptySOCProof, err
	}

	var signature [65]byte
	copy(signature[:], socCh.Signature())
	var signer [20]byte
	copy(signer[:], socCh.OwnerAddress())

	return [1]SOCProof{{
		Signer:     signer,
		Signature:  signature,
		Identifier: common.BytesToHash(socCh.ID()),
		ChunkAddr:  common.BytesToHash(socCh.WrappedChunk().Address().Bytes()),
	}}, nil
}
