// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	"fmt"
	"math/big"

	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/bmtpool"
	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/storageincentives/redistribution"
	"github.com/ethersphere/bee/pkg/storageincentives/types"
	. "github.com/ethersphere/bee/pkg/storageincentives/types"
	"github.com/ethersphere/bee/pkg/storer"
	"github.com/ethersphere/bee/pkg/swarm"
)

func makeInclusionProofs(
	reserveSampleItems []storer.SampleItem,
	anchor1 []byte,
	anchor2 []byte,
) (redistribution.ChunkInclusionProofs, error) {
	proofs := redistribution.ChunkInclusionProofs{}

	if len(reserveSampleItems) != 16 {
		return proofs, fmt.Errorf("reserve sample items should have 16 elements")
	}
	if len(anchor1) == 0 {
		return proofs, fmt.Errorf("anchor1 is not set")
	}
	if len(anchor2) == 0 {
		return proofs, fmt.Errorf("anchor2 is not set")
	}

	rsc, err := sampleChunk(reserveSampleItems)
	if err != nil {
		return proofs, fmt.Errorf("failed to make sample chunk: %w", err)
	}
	rscData := rsc.Data()

	hasher, trHasher := bmtpool.Get(), bmt.NewTrHasher(anchor1)
	defer bmtpool.Put(hasher)

	anchor2Big := new(big.Int).SetBytes(anchor2)
	segmentIndex := int(new(big.Int).Mod(anchor2Big, big.NewInt(128)).Int64())

	wcp := Trio[int]{} // wcp := witness chunk position
	wcp.A = int(new(big.Int).Mod(anchor2Big, big.NewInt(15)).Int64())
	wcp.B = int(new(big.Int).Mod(anchor2Big, big.NewInt(14)).Int64())
	wcp.C = 15

	wc := Trio[storer.SampleItem]{} // wc = witness chunk
	wc.A = reserveSampleItems[wcp.A]
	wc.B = reserveSampleItems[wcp.B]
	wc.C = reserveSampleItems[wcp.C]

	wp, err := makeProofTrio( // wp := witness chunks proof
		hasher,
		types.NewTrio(rscData, rscData, rscData),
		types.NewTrio(2*wcp.A, 2*wcp.B, 2*wcp.C),
	)
	if err != nil {
		return proofs, fmt.Errorf("witness chunk proofs: %w", err)
	}

	rp, err := makeProofTrio( // rp = retention proofs
		hasher,
		types.NewTrio(wc.A.ChunkData, wc.B.ChunkData, wc.C.ChunkData),
		types.NewTrio(segmentIndex, segmentIndex, segmentIndex),
	)
	if err != nil {
		return proofs, fmt.Errorf("retention proofs: %w", err)
	}

	trp, err := makeProofTrio( //  trp = transformed address proofs
		trHasher,
		types.NewTrio(wc.A.ChunkData, wc.B.ChunkData, wc.C.ChunkData),
		types.NewTrio(segmentIndex, segmentIndex, segmentIndex),
	)
	if err != nil {
		return proofs, fmt.Errorf("transformed address proofs: %w", err)
	}

	proofs.A, err = newChunkInclusionProof(wp.A, rp.A, trp.A, wc.A.ChunkAddress, wc.A.Stamp)
	if err != nil {
		return proofs, fmt.Errorf("chunk inclusion proof #1: %w", err)
	}

	proofs.B, err = newChunkInclusionProof(wp.B, rp.B, trp.B, wc.B.ChunkAddress, wc.B.Stamp)
	if err != nil {
		return proofs, fmt.Errorf("chunk inclusion proof #2: %w", err)
	}

	proofs.C, err = newChunkInclusionProof(wp.C, rp.C, trp.C, wc.C.ChunkAddress, wc.C.Stamp)
	if err != nil {
		return proofs, fmt.Errorf("chunk inclusion proof #3: %w", err)
	}

	return proofs, nil
}

func makeProofTrio(
	h *bmt.Hasher,
	data types.Trio[[]byte],
	segmentIndex types.Trio[int],
) (types.Trio[bmt.Proof], error) {
	var err error
	proof := Trio[bmt.Proof]{}

	proof.A, err = makeProof(h, data.A, segmentIndex.A)
	if err != nil {
		return proof, fmt.Errorf("make proof #1: %w", err)
	}

	proof.B, err = makeProof(h, data.B, segmentIndex.B)
	if err != nil {
		return proof, fmt.Errorf("make proof #2: %w", err)
	}

	proof.C, err = makeProof(h, data.C, segmentIndex.C)
	if err != nil {
		return proof, fmt.Errorf("make proof #3: %w", err)
	}

	return proof, nil
}

func makeProof(h *bmt.Hasher, data []byte, i int) (bmt.Proof, error) {
	h.Reset()
	h.SetHeader(data[:swarm.SpanSize])
	_, err := h.Write(data[swarm.SpanSize:])
	if err != nil {
		return bmt.Proof{}, err
	}
	_, err = h.Hash(nil)
	if err != nil {
		return bmt.Proof{}, err
	}

	p := bmt.Prover{Hasher: h}
	return p.Proof(i), nil
}

func sampleChunk(items []storer.SampleItem) (swarm.Chunk, error) {
	contentSize := len(items) * 2 * swarm.HashSize

	pos := 0
	content := make([]byte, contentSize)
	for _, s := range items {
		copy(content[pos:], s.ChunkAddress.Bytes())
		pos += swarm.HashSize
		copy(content[pos:], s.TransformedAddress.Bytes())
		pos += swarm.HashSize
	}

	return cac.New(content)
}

func sampleHash(items []storer.SampleItem) (swarm.Address, error) {
	ch, err := sampleChunk(items)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	return ch.Address(), nil
}

func newChunkInclusionProof(
	proofp1, proofp2, proofp3 bmt.Proof,
	chunkAddress swarm.Address,
	stamp swarm.Stamp,
) (redistribution.ChunkInclusionProof, error) {
	proofp1Hex := bytesToHex(proofp1)
	proofp2Hex := bytesToHex(proofp2)
	proofp3Hex := bytesToHex(proofp3)

	// batchOwner, err := postage.RecoverBatchOwner(chunkAddress, stamp)
	// if err != nil {
	// 	return redistribution.ChunkInclusionProof{}, err
	// }

	return redistribution.ChunkInclusionProof{
		ProofSegments:  proofp1Hex.ProofSegments,
		ProveSegment:   proofp1Hex.ProveSegment,
		ProofSegments2: proofp2Hex.ProofSegments,
		ProveSegment2:  proofp2Hex.ProveSegment,
		ChunkSpan:      bmt.LengthFromSpan(proofp2.Span),
		ProofSegments3: proofp3Hex.ProofSegments,
		Signature:      stamp.Sig(),
		ChunkAddr:      types.ToByte32(chunkAddress.Bytes()),
		PostageId:      types.ToByte32(stamp.BatchID()),
		Index:          stamp.Index(),
		TimeStamp:      stamp.Timestamp(),
	}, nil
}

func bytesToHex(proof bmt.Proof) hexProof {
	var proveSegment []byte

	proofSegments := make([][32]byte, len(proof.ProofSegments)+1)
	if proof.Index%2 == 0 {
		proofSegments[0] = types.ToByte32(proof.ProveSegment[swarm.SectionSize:])
		proveSegment = proof.ProveSegment[:swarm.SectionSize]
	} else {
		proofSegments[0] = types.ToByte32(proof.ProveSegment[:swarm.SectionSize])
		proveSegment = proof.ProveSegment[swarm.SectionSize:]
	}
	for i := 0; i < len(proof.ProofSegments); i++ {
		proofSegments[i+1] = types.ToByte32(proof.ProofSegments[i])
	}

	return hexProof{
		ProveSegment:  types.ToByte32(proveSegment),
		ProofSegments: proofSegments,
	}
}

type hexProof struct {
	ProofSegments [][32]byte
	ProveSegment  [32]byte
}
