// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	"errors"
	"fmt"
	"hash"
	"math/big"

	"github.com/ethersphere/bee/v2/pkg/bmt"
	"github.com/ethersphere/bee/v2/pkg/bmtpool"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storageincentives/redistribution"
	storer "github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var errProofCreation = errors.New("reserve commitment hasher: failure in proof creation")

// spanOffset returns the byte index of chunkdata where the spansize starts
func spanOffset(sampleItem storer.SampleItem) uint8 {
	ch := swarm.NewChunk(sampleItem.ChunkAddress, sampleItem.ChunkData)
	if soc.Valid(ch) {
		return swarm.HashSize + swarm.SocSignatureSize
	}

	return 0
}

// makeInclusionProofs creates transaction data for claim method.
// In the document this logic, result data, is also called Proof of entitlement (POE).
func makeInclusionProofs(
	reserveSampleItems []storer.SampleItem,
	anchor1 []byte,
	anchor2 []byte,
) (redistribution.ChunkInclusionProofs, error) {
	if len(reserveSampleItems) != storer.SampleSize {
		return redistribution.ChunkInclusionProofs{}, fmt.Errorf("reserve sample items should have %d elements", storer.SampleSize)
	}
	if len(anchor1) == 0 {
		return redistribution.ChunkInclusionProofs{}, errors.New("anchor1 is not set")
	}
	if len(anchor2) == 0 {
		return redistribution.ChunkInclusionProofs{}, errors.New("anchor2 is not set")
	}

	require3 := storer.SampleSize - 1
	require1 := new(big.Int).Mod(new(big.Int).SetBytes(anchor2), big.NewInt(int64(require3))).Uint64()
	require2 := new(big.Int).Mod(new(big.Int).SetBytes(anchor2), big.NewInt(int64(require3-1))).Uint64()
	if require2 >= require1 {
		require2++
	}

	prefixHasherFactory := func() hash.Hash {
		return swarm.NewPrefixHasher(anchor1)
	}
	prefixHasherPool := bmt.NewPool(bmt.NewConf(prefixHasherFactory, swarm.BmtBranches, 8))

	// Sample chunk proofs
	rccontent := bmt.Prover{Hasher: bmtpool.Get()}
	rccontent.SetHeaderInt64(swarm.HashSize * storer.SampleSize * 2)
	rsc, err := sampleChunk(reserveSampleItems)
	if err != nil {
		return redistribution.ChunkInclusionProofs{}, errProofCreation
	}
	rscData := rsc.Data()
	_, err = rccontent.Write(rscData[swarm.SpanSize:])
	if err != nil {
		return redistribution.ChunkInclusionProofs{}, errProofCreation
	}
	_, err = rccontent.Hash(nil)
	if err != nil {
		return redistribution.ChunkInclusionProofs{}, errProofCreation
	}
	proof1p1 := rccontent.Proof(int(require1) * 2)
	proof2p1 := rccontent.Proof(int(require2) * 2)
	proofLastp1 := rccontent.Proof(require3 * 2)
	bmtpool.Put(rccontent.Hasher)

	// Witness1 proofs
	segmentIndex := int(new(big.Int).Mod(new(big.Int).SetBytes(anchor2), big.NewInt(int64(128))).Uint64())
	// OG chunk proof
	chunk1Content := bmt.Prover{Hasher: bmtpool.Get()}
	chunk1Offset := spanOffset(reserveSampleItems[require1])
	chunk1Content.SetHeader(reserveSampleItems[require1].ChunkData[chunk1Offset : chunk1Offset+swarm.SpanSize])
	chunk1ContentPayload := reserveSampleItems[require1].ChunkData[chunk1Offset+swarm.SpanSize:]
	_, err = chunk1Content.Write(chunk1ContentPayload)
	if err != nil {
		return redistribution.ChunkInclusionProofs{}, errProofCreation
	}
	_, err = chunk1Content.Hash(nil)
	if err != nil {
		return redistribution.ChunkInclusionProofs{}, errProofCreation
	}
	proof1p2 := chunk1Content.Proof(segmentIndex)
	// TR chunk proof
	chunk1TrContent := bmt.Prover{Hasher: prefixHasherPool.Get()}
	chunk1TrContent.SetHeader(reserveSampleItems[require1].ChunkData[chunk1Offset : chunk1Offset+swarm.SpanSize])
	_, err = chunk1TrContent.Write(chunk1ContentPayload)
	if err != nil {
		return redistribution.ChunkInclusionProofs{}, errProofCreation
	}
	_, err = chunk1TrContent.Hash(nil)
	if err != nil {
		return redistribution.ChunkInclusionProofs{}, errProofCreation
	}
	proof1p3 := chunk1TrContent.Proof(segmentIndex)
	// cleanup
	bmtpool.Put(chunk1Content.Hasher)
	prefixHasherPool.Put(chunk1TrContent.Hasher)

	// Witness2 proofs
	// OG Chunk proof
	chunk2Offset := spanOffset(reserveSampleItems[require2])
	chunk2Content := bmt.Prover{Hasher: bmtpool.Get()}
	chunk2ContentPayload := reserveSampleItems[require2].ChunkData[chunk2Offset+swarm.SpanSize:]
	chunk2Content.SetHeader(reserveSampleItems[require2].ChunkData[chunk2Offset : chunk2Offset+swarm.SpanSize])
	_, err = chunk2Content.Write(chunk2ContentPayload)
	if err != nil {
		return redistribution.ChunkInclusionProofs{}, errProofCreation
	}
	_, err = chunk2Content.Hash(nil)
	if err != nil {
		return redistribution.ChunkInclusionProofs{}, errProofCreation
	}
	proof2p2 := chunk2Content.Proof(segmentIndex)
	// TR Chunk proof
	chunk2TrContent := bmt.Prover{Hasher: prefixHasherPool.Get()}
	chunk2TrContent.SetHeader(reserveSampleItems[require2].ChunkData[chunk2Offset : chunk2Offset+swarm.SpanSize])
	_, err = chunk2TrContent.Write(chunk2ContentPayload)
	if err != nil {
		return redistribution.ChunkInclusionProofs{}, errProofCreation
	}
	_, err = chunk2TrContent.Hash(nil)
	if err != nil {
		return redistribution.ChunkInclusionProofs{}, errProofCreation
	}
	proof2p3 := chunk2TrContent.Proof(segmentIndex)
	// cleanup
	bmtpool.Put(chunk2Content.Hasher)
	prefixHasherPool.Put(chunk2TrContent.Hasher)

	// Witness3 proofs
	// OG Chunk proof
	chunkLastOffset := spanOffset(reserveSampleItems[require3])
	chunkLastContent := bmt.Prover{Hasher: bmtpool.Get()}
	chunkLastContent.SetHeader(reserveSampleItems[require3].ChunkData[chunkLastOffset : chunkLastOffset+swarm.SpanSize])
	chunkLastContentPayload := reserveSampleItems[require3].ChunkData[chunkLastOffset+swarm.SpanSize:]
	_, err = chunkLastContent.Write(chunkLastContentPayload)
	if err != nil {
		return redistribution.ChunkInclusionProofs{}, errProofCreation
	}
	_, err = chunkLastContent.Hash(nil)
	if err != nil {
		return redistribution.ChunkInclusionProofs{}, errProofCreation
	}
	proofLastp2 := chunkLastContent.Proof(segmentIndex)
	// TR Chunk Proof
	chunkLastTrContent := bmt.Prover{Hasher: prefixHasherPool.Get()}
	chunkLastTrContent.SetHeader(reserveSampleItems[require3].ChunkData[chunkLastOffset : chunkLastOffset+swarm.SpanSize])
	_, err = chunkLastTrContent.Write(chunkLastContentPayload)
	if err != nil {
		return redistribution.ChunkInclusionProofs{}, errProofCreation
	}
	_, err = chunkLastTrContent.Hash(nil)
	if err != nil {
		return redistribution.ChunkInclusionProofs{}, errProofCreation
	}
	proofLastp3 := chunkLastTrContent.Proof(segmentIndex)
	// cleanup
	bmtpool.Put(chunkLastContent.Hasher)
	prefixHasherPool.Put(chunkLastTrContent.Hasher)

	// map to output and add SOC related data if it is necessary
	A, err := redistribution.NewChunkInclusionProof(proof1p1, proof1p2, proof1p3, reserveSampleItems[require1])
	if err != nil {
		return redistribution.ChunkInclusionProofs{}, err
	}
	B, err := redistribution.NewChunkInclusionProof(proof2p1, proof2p2, proof2p3, reserveSampleItems[require2])
	if err != nil {
		return redistribution.ChunkInclusionProofs{}, err
	}
	C, err := redistribution.NewChunkInclusionProof(proofLastp1, proofLastp2, proofLastp3, reserveSampleItems[require3])
	if err != nil {
		return redistribution.ChunkInclusionProofs{}, err
	}
	return redistribution.ChunkInclusionProofs{
		A: A,
		B: B,
		C: C,
	}, nil
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
