// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	"fmt"

	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/bmtpool"
	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/storageincentives/redistribution"
	. "github.com/ethersphere/bee/pkg/storageincentives/types"
	"github.com/ethersphere/bee/pkg/storer"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	minSampleLength = 2
	saltLength      = 8
)

func makeInclusionProofs(
	reserveSampleItems []storer.SampleItem,
	salt []byte,
) (redistribution.ChunkInclusionProofs, error) {
	proofs := redistribution.ChunkInclusionProofs{}

	sampleLength := len(reserveSampleItems)
	if sampleLength < minSampleLength {
		return proofs, fmt.Errorf("reserve sample items should not have less then %d elements", minSampleLength)
	}

	if len(salt) != saltLength {
		return proofs, fmt.Errorf("salt should be exactly length of %d", saltLength)
	}

	rsc, err := sampleChunk(reserveSampleItems)
	if err != nil {
		return proofs, fmt.Errorf("failed to make sample chunk: %w", err)
	}
	rscData := rsc.Data()

	hasher, trHasher := bmtpool.Get(), bmt.NewTrHasher(salt)
	defer bmtpool.Put(hasher)

	ssn := segmentSelection(salt)

	wcp := Trio[int]{} // wcp = witness chunk position
	wcp.Element1 = ssn.Element1 % (sampleLength - 1)
	wcp.Element2 = ssn.Element2 % (sampleLength - 2)
	if wcp.Element2 == wcp.Element1 {
		wcp.Element2 = sampleLength - 2
	}
	wcp.Element3 = sampleLength - 2

	wc := Trio[storer.SampleItem]{} // wc = witness chunk
	wc.Element1 = reserveSampleItems[wcp.Element1]
	wc.Element2 = reserveSampleItems[wcp.Element2]
	wc.Element3 = reserveSampleItems[wcp.Element3]

	wp := Trio[bmt.Proof]{} // wp := witness chunks proof
	wp.Element1 = makeProof(hasher, rscData, 2*wcp.Element1)
	wp.Element2 = makeProof(hasher, rscData, 2*wcp.Element2)
	wp.Element3 = makeProof(hasher, rscData, 2*wcp.Element3)

	rp := Trio[bmt.Proof]{} // rp = retention proofs
	rp.Element1 = makeProof(hasher, wc.Element1.ChunkData, ssn.Element1%128)
	rp.Element2 = makeProof(hasher, wc.Element2.ChunkData, ssn.Element2%128)
	rp.Element3 = makeProof(hasher, wc.Element3.ChunkData, ssn.Element3%128)

	trp := Trio[bmt.Proof]{} // trp = transformed address proofs
	trp.Element1 = makeProof(trHasher, wc.Element1.ChunkData, ssn.Element1%128)
	trp.Element2 = makeProof(trHasher, wc.Element2.ChunkData, ssn.Element2%128)
	trp.Element3 = makeProof(trHasher, wc.Element3.ChunkData, ssn.Element3%128)

	proofs.Element1 = redistribution.ChunkInclusionProof{
		PostageStamp:            wc.Element1.Stamp,
		WitnessProof:            wp.Element1,
		RetentionProof:          rp.Element1,
		TransformedAddressProof: trp.Element1,
	}
	proofs.Element2 = redistribution.ChunkInclusionProof{
		PostageStamp:            wc.Element2.Stamp,
		WitnessProof:            wp.Element2,
		RetentionProof:          rp.Element2,
		TransformedAddressProof: trp.Element2,
	}
	proofs.Element3 = redistribution.ChunkInclusionProof{
		PostageStamp:            wc.Element3.Stamp,
		WitnessProof:            wp.Element3,
		RetentionProof:          rp.Element3,
		TransformedAddressProof: trp.Element3,
	}

	return proofs, nil
}

func makeProof(h *bmt.Hasher, data []byte, i int) bmt.Proof {
	h.Reset()
	_, _ = h.Write(data)
	_, _ = h.Hash(nil)

	p := bmt.Prover{Hasher: h}
	return p.Proof(i)
}

func segmentSelection(salt []byte) Trio[int] {
	// TODO
	// SSN(g,i) = H(R(g)|BE_8(i))

	// bi := big.NewInt(0).SetBytes(salt)

	be := int64(salt[0])
	be = be << 8

	ssn := func(i int64) int {
		return int(be | i)
	}

	return Trio[int]{
		Element1: ssn(0),
		Element2: ssn(1),
		Element3: ssn(2),
	}
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
