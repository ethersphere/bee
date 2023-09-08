// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/bmtpool"
	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/storageincentives/redistribution"
	"github.com/ethersphere/bee/pkg/storageincentives/types"
	"github.com/ethersphere/bee/pkg/storer"
	"github.com/ethersphere/bee/pkg/swarm"
)

// makeInclusionProofs creates transaction data for claim method.
// In the document this logic, result data, is also called Proof of entitlement (POE).
func makeInclusionProofs(
	reserveSampleItems []storer.SampleItem,
	anchor1 []byte,
	anchor2 []byte,
) (redistribution.ChunkInclusionProofs, error) {
	proofs := redistribution.ChunkInclusionProofs{}

	if len(reserveSampleItems) != storer.SampleSize {
		return proofs, fmt.Errorf("reserve sample items should have %d elements", storer.SampleSize)
	}
	if len(anchor1) == 0 {
		return proofs, errors.New("anchor1 is not set")
	}
	if len(anchor2) == 0 {
		return proofs, errors.New("anchor2 is not set")
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

	wcp := types.Trio[int]{} // wcp := witness chunk position
	wcp.A = int(new(big.Int).Mod(anchor2Big, big.NewInt(storer.SampleSize-1)).Int64())
	wcp.B = int(new(big.Int).Mod(anchor2Big, big.NewInt(storer.SampleSize-2)).Int64())
	if wcp.B >= wcp.A {
		wcp.B++
	}
	wcp.C = storer.SampleSize - 1

	wc := types.Trio[storer.SampleItem]{} // wc = witness chunk
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

	wcData := types.NewTrio(wc.A.ChunkData, wc.B.ChunkData, wc.C.ChunkData)
	segmentIndexTrio := types.NewTrio(segmentIndex, segmentIndex, segmentIndex)

	// rp = retention proofs
	rp, err := makeProofTrio(hasher, wcData, segmentIndexTrio)
	if err != nil {
		return proofs, fmt.Errorf("retention proofs: %w", err)
	}

	//  trp = transformed address proofs
	trp, err := makeProofTrio(trHasher, wcData, segmentIndexTrio)
	if err != nil {
		return proofs, fmt.Errorf("transformed address proofs: %w", err)
	}

	proofs.A, err = newChunkInclusionProof(wp.A, rp.A, trp.A, wc.A)
	if err != nil {
		return proofs, fmt.Errorf("chunk inclusion proof #1: %w", err)
	}

	proofs.B, err = newChunkInclusionProof(wp.B, rp.B, trp.B, wc.B)
	if err != nil {
		return proofs, fmt.Errorf("chunk inclusion proof #2: %w", err)
	}

	proofs.C, err = newChunkInclusionProof(wp.C, rp.C, trp.C, wc.C)
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
	proof := types.Trio[bmt.Proof]{}
	dataA := make([]byte, swarm.ChunkSize)
	copy(dataA, data.A)
	dataB := make([]byte, swarm.ChunkSize)
	copy(dataB, data.B)
	dataC := make([]byte, swarm.ChunkSize)
	copy(dataC, data.C)

	proof.A, err = makeProof(h, dataA, segmentIndex.A)
	if err != nil {
		return proof, fmt.Errorf("make proof #1: %w", err)
	}

	proof.B, err = makeProof(h, dataB, segmentIndex.B)
	if err != nil {
		return proof, fmt.Errorf("make proof #2: %w", err)
	}

	proof.C, err = makeProof(h, dataC, segmentIndex.C)
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

func newChunkInclusionProof(
	proofp1, proofp2, proofp3 bmt.Proof,
	sampleItem storer.SampleItem,
) (redistribution.ChunkInclusionProof, error) {
	proofp1Hex := bytesToHex(proofp1)
	proofp2Hex := bytesToHex(proofp2)
	proofp3Hex := bytesToHex(proofp3)

	socProof, err := makeSOCProof(sampleItem)
	if err != nil {
		return redistribution.ChunkInclusionProof{}, err
	}

	return redistribution.ChunkInclusionProof{
		ProofSegments:    proofp1Hex.ProofSegments,
		ProveSegment:     types.ToHexString(sampleItem.ChunkAddress.Bytes()),
		ProofSegments2:   proofp2Hex.ProofSegments,
		ProveSegment2:    proofp2Hex.ProveSegment,
		ChunkSpan:        bmt.LengthFromSpan(proofp2.Span),
		ProofSegments3:   proofp3Hex.ProofSegments,
		Signature:        hex.EncodeToString(sampleItem.Stamp.Sig()),
		ChunkAddr:        types.ToHexString(sampleItem.ChunkAddress.Bytes()),
		PostageId:        types.ToHexString(sampleItem.Stamp.BatchID()),
		Index:            hex.EncodeToString(sampleItem.Stamp.Index()),
		TimeStamp:        hex.EncodeToString(sampleItem.Stamp.Timestamp()),
		SocProofAttached: socProof,
	}, nil
}

func bytesToHex(proof bmt.Proof) hexProof {
	var proveSegment []byte

	proofSegments := make([]string, len(proof.ProofSegments)+1)
	if proof.Index%2 == 0 {
		proofSegments[0] = types.ToHexString(proof.ProveSegment[swarm.SectionSize:])
		proveSegment = proof.ProveSegment[:swarm.SectionSize]
	} else {
		proofSegments[0] = types.ToHexString(proof.ProveSegment[:swarm.SectionSize])
		proveSegment = proof.ProveSegment[swarm.SectionSize:]
	}
	for i := 0; i < len(proof.ProofSegments); i++ {
		proofSegments[i+1] = types.ToHexString(proof.ProofSegments[i])
	}

	return hexProof{
		ProveSegment:  types.ToHexString(proveSegment),
		ProofSegments: proofSegments,
	}
}

type hexProof struct {
	ProofSegments []string
	ProveSegment  string
}

func makeSOCProof(sampleItem storer.SampleItem) ([]redistribution.SOCProof, error) {
	var emptySOCProof = make([]redistribution.SOCProof, 0)
	ch := swarm.NewChunk(sampleItem.ChunkAddress, sampleItem.ChunkData)
	if !soc.Valid(ch) {
		return emptySOCProof, nil
	}

	socCh, err := soc.FromChunk(ch)
	if err != nil {
		return emptySOCProof, err
	}

	return []redistribution.SOCProof{{
		Signer:     common.Address(socCh.OwnerAddress()),
		Signature:  hex.EncodeToString(socCh.Signature()),
		Identifier: types.ToHexString(socCh.ID()),
		ChunkAddr:  types.ToHexString(socCh.WrappedChunk().Address().Bytes()),
	}}, nil
}

// sampleChunk creates reserve sample chunk (RSC) from supplied sample items.
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

// sampleHash creates hash form RSC.
func sampleHash(items []storer.SampleItem) (swarm.Address, error) {
	ch, err := sampleChunk(items)
	if err != nil {
		return swarm.ZeroAddress, err
	}
	return ch.Address(), nil
}
