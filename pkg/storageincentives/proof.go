// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/bmtpool"
	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/postage"
	"github.com/ethersphere/bee/pkg/storageincentives/redistribution"
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

	wcp := Trio[int]{}
	wcp.Element1 = int(new(big.Int).Mod(new(big.Int).SetBytes(anchor2), big.NewInt(15)).Int64())
	wcp.Element2 = int(new(big.Int).Mod(new(big.Int).SetBytes(anchor2), big.NewInt(14)).Int64())
	wcp.Element3 = 15

	segmentIndex := int(new(big.Int).Mod(new(big.Int).SetBytes(anchor2), big.NewInt(int64(128))).Int64())

	wc := Trio[storer.SampleItem]{} // wc = witness chunk
	wc.Element1 = reserveSampleItems[wcp.Element1]
	wc.Element2 = reserveSampleItems[wcp.Element2]
	wc.Element3 = reserveSampleItems[wcp.Element3]

	wp := Trio[bmt.Proof]{} // wp := witness chunks proof
	wp.Element1, _ = makeProof(hasher, rscData, 2*wcp.Element1)
	wp.Element2, _ = makeProof(hasher, rscData, 2*wcp.Element2)
	wp.Element3, _ = makeProof(hasher, rscData, 2*wcp.Element3)

	rp := Trio[bmt.Proof]{} // rp = retention proofs
	rp.Element1, _ = makeProof(hasher, wc.Element1.ChunkData, segmentIndex)
	rp.Element2, _ = makeProof(hasher, wc.Element2.ChunkData, segmentIndex)
	rp.Element3, _ = makeProof(hasher, wc.Element3.ChunkData, segmentIndex)

	trp := Trio[bmt.Proof]{} // trp = transformed address proofs
	trp.Element1, _ = makeProof(trHasher, wc.Element1.ChunkData, segmentIndex)
	trp.Element2, _ = makeProof(trHasher, wc.Element2.ChunkData, segmentIndex)
	trp.Element3, _ = makeProof(trHasher, wc.Element3.ChunkData, segmentIndex)

	proofs.Element1, err = newChunkInclusionProof(wp.Element1, rp.Element1, trp.Element1, wc.Element1.ChunkAddress, wc.Element1.Stamp)
	if err != nil {
		return proofs, err
	}
	proofs.Element2, err = newChunkInclusionProof(wp.Element2, rp.Element2, trp.Element2, wc.Element2.ChunkAddress, wc.Element2.Stamp)
	if err != nil {
		return proofs, err
	}
	proofs.Element3, err = newChunkInclusionProof(wp.Element3, rp.Element3, trp.Element3, wc.Element3.ChunkAddress, wc.Element3.Stamp)
	if err != nil {
		return proofs, err
	}

	return proofs, nil
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

	batchOwner, err := postage.RecoverBatchOwner(chunkAddress, stamp)
	if err != nil {
		return redistribution.ChunkInclusionProof{}, err
	}

	return redistribution.ChunkInclusionProof{
		ProofSegments:  proofp1Hex.ProofSegments,
		ProveSegment:   proofp1Hex.ProveSegment,
		ProofSegments2: proofp2Hex.ProofSegments,
		ProveSegment2:  proofp2Hex.ProveSegment,
		ChunkSpan:      binary.LittleEndian.Uint64(proofp2.Span[:swarm.SpanSize]),
		ProofSegments3: proofp3Hex.ProofSegments,
		Signer:         batchOwner,
		Signature:      stamp.Sig(),
		ChunkAddr:      chunkAddress.Bytes(),
		PostageId:      stamp.BatchID(),
		Index:          stamp.Index(),
		TimeStamp:      stamp.Timestamp(),
	}, nil
}

func bytesToHex(proof bmt.Proof) hexProof {
	var proveSegment []byte

	proofSegments := make([][]byte, len(proof.ProofSegments)+1)
	if proof.Index%2 == 0 {
		proofSegments[0] = proof.ProveSegment[swarm.SectionSize:]
		proveSegment = proof.ProveSegment[:swarm.SectionSize]
	} else {
		proofSegments[0] = proof.ProveSegment[:swarm.SectionSize]
		proveSegment = proof.ProveSegment[swarm.SectionSize:]
	}
	for i, sister := range proof.ProofSegments {
		proofSegments[i+1] = sister
	}

	return hexProof{ProveSegment: proveSegment, ProofSegments: proofSegments}
}

type hexProof struct {
	ProofSegments [][]byte
	ProveSegment  []byte
}
