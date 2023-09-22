// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Used for inclusion proof utilities

package redistribution

import (
	"encoding/binary"
	"encoding/hex"

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
	TimeStamp string `json:"timeStamp"`

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

// Transforms arguments to ChunkInclusionProof object
func NewChunkInclusionProof(proofp1, proofp2 bmt.Proof, proofp3 bmt.Proof, sampleItem storer.SampleItem) (ChunkInclusionProof, error) {
	proofp1Hex := newHexProofs(proofp1)
	proofp2Hex := newHexProofs(proofp2)
	proofp3Hex := newHexProofs(proofp3)

	socProof, err := makeSOCProof(sampleItem)
	if err != nil {
		return ChunkInclusionProof{}, err
	}

	return ChunkInclusionProof{
		ProofSegments:    proofp1Hex.ProofSegments,
		ProveSegment:     proofp1Hex.ProveSegment,
		ProofSegments2:   proofp2Hex.ProofSegments,
		ProveSegment2:    proofp2Hex.ProveSegment,
		ChunkSpan:        binary.LittleEndian.Uint64(proofp2.Span[:swarm.SpanSize]), // should be uint64 on the other size; copied from pkg/api/bytes.go
		ProofSegments3:   proofp3Hex.ProofSegments,
		Signature:        hex.EncodeToString(sampleItem.Stamp.Sig()),
		ChunkAddr:        proofp1Hex.ProveSegment, // TODO refactor ABI
		PostageId:        hex.EncodeToString(sampleItem.Stamp.BatchID()),
		Index:            hex.EncodeToString(sampleItem.Stamp.Index()),
		TimeStamp:        hex.EncodeToString(sampleItem.Stamp.Timestamp()),
		SocProofAttached: socProof,
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
		Signer:     common.Address(socCh.OwnerAddress()),
		Signature:  hex.EncodeToString(socCh.Signature()),
		Identifier: hex.EncodeToString(socCh.ID()),
		ChunkAddr:  hex.EncodeToString(socCh.WrappedChunk().Address().Bytes()),
	}}, nil
}

type hexProof struct {
	ProofSegments []string
	ProveSegment  string
}

// Transforms proof object to its hexadecimal representation
func newHexProofs(proof bmt.Proof) hexProof {
	proofSegments := make([]string, len(proof.ProofSegments))
	for i := 0; i < len(proof.ProofSegments); i++ {
		proofSegments[i] = hex.EncodeToString(proof.ProofSegments[i])
	}

	return hexProof{
		ProveSegment:  hex.EncodeToString(proof.ProveSegment),
		ProofSegments: proofSegments,
	}
}
