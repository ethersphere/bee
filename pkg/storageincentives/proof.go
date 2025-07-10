// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	"errors"
	"fmt"
	"hash"
	"math/big"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/bmt"
	"github.com/ethersphere/bee/v2/pkg/bmtpool"
	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/soc"
	"github.com/ethersphere/bee/v2/pkg/storageincentives/redistribution"
	storer "github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var errProofCreation = errors.New("reserve commitment hasher: failure in proof creation")

// Pool for reusing content buffers to reduce allocations
var contentBufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, storer.SampleSize*2*swarm.HashSize)
		return &buf
	},
}

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

	// Pre-compute big.Int values to avoid repeated allocations
	anchor2BigInt := new(big.Int).SetBytes(anchor2)
	require3BigInt := big.NewInt(int64(require3))
	require3Minus1BigInt := big.NewInt(int64(require3 - 1))
	segmentIndexBigInt := big.NewInt(int64(128))

	require1 := new(big.Int).Mod(anchor2BigInt, require3BigInt).Uint64()
	require2 := new(big.Int).Mod(anchor2BigInt, require3Minus1BigInt).Uint64()
	if require2 >= require1 {
		require2++
	}

	// Pre-compute segment index once
	segmentIndex := int(new(big.Int).Mod(anchor2BigInt, segmentIndexBigInt).Uint64())

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
	// (segmentIndex already computed above)
	proof1p2, proof1p3, err := processChunkProof(reserveSampleItems[require1], segmentIndex, prefixHasherPool)
	if err != nil {
		return redistribution.ChunkInclusionProofs{}, err
	}

	// Witness2 proofs
	proof2p2, proof2p3, err := processChunkProof(reserveSampleItems[require2], segmentIndex, prefixHasherPool)
	if err != nil {
		return redistribution.ChunkInclusionProofs{}, err
	}

	// Witness3 proofs
	proofLastp2, proofLastp3, err := processChunkProof(reserveSampleItems[require3], segmentIndex, prefixHasherPool)
	if err != nil {
		return redistribution.ChunkInclusionProofs{}, err
	}

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

// Helper function to process chunk proofs, reducing code duplication
func processChunkProof(
	sampleItem storer.SampleItem,
	segmentIndex int,
	prefixHasherPool *bmt.Pool,
) (ogProof, trProof bmt.Proof, err error) {
	offset := spanOffset(sampleItem)

	// OG chunk proof
	ogContent := bmt.Prover{Hasher: bmtpool.Get()}
	defer bmtpool.Put(ogContent.Hasher)

	ogContent.SetHeader(sampleItem.ChunkData[offset : offset+swarm.SpanSize])
	payload := sampleItem.ChunkData[offset+swarm.SpanSize:]

	if _, err = ogContent.Write(payload); err != nil {
		return bmt.Proof{}, bmt.Proof{}, errProofCreation
	}
	if _, err = ogContent.Hash(nil); err != nil {
		return bmt.Proof{}, bmt.Proof{}, errProofCreation
	}
	ogProof = ogContent.Proof(segmentIndex)

	// TR chunk proof
	trContent := bmt.Prover{Hasher: prefixHasherPool.Get()}
	defer prefixHasherPool.Put(trContent.Hasher)

	trContent.SetHeader(sampleItem.ChunkData[offset : offset+swarm.SpanSize])
	if _, err = trContent.Write(payload); err != nil {
		return bmt.Proof{}, bmt.Proof{}, errProofCreation
	}
	if _, err = trContent.Hash(nil); err != nil {
		return bmt.Proof{}, bmt.Proof{}, errProofCreation
	}
	trProof = trContent.Proof(segmentIndex)

	return ogProof, trProof, nil
}

func sampleChunk(items []storer.SampleItem) (swarm.Chunk, error) {
	contentSize := len(items) * 2 * swarm.HashSize

	// Get buffer from pool to reduce allocations
	contentPtr := contentBufferPool.Get().(*[]byte)
	defer contentBufferPool.Put(contentPtr)

	content := *contentPtr
	if cap(content) < contentSize {
		content = make([]byte, contentSize)
	} else {
		content = content[:contentSize]
	}

	pos := 0
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
