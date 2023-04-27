// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storageincentives

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/bmtpool"
	"github.com/ethersphere/bee/pkg/cac"
	"github.com/ethersphere/bee/pkg/storageincentives/redistribution"
	. "github.com/ethersphere/bee/pkg/storageincentives/types"
	"github.com/ethersphere/bee/pkg/storer"
	"github.com/ethersphere/bee/pkg/swarm"
)

const minSampleLength = 2

func makeInclusionProofs(ctx context.Context, sampleData SampleData) (redistribution.ChunkInclusionProofs, error) {
	proofs := redistribution.ChunkInclusionProofs{}

	rsi := sampleData.ReserveSampleItems
	sampleLength := len(rsi)
	if sampleLength < minSampleLength {
		return proofs, fmt.Errorf("reserve sample items should not have less then %d elements", minSampleLength)
	}

	rsc, err := sampleChunk(rsi)
	if err != nil {
		return proofs, fmt.Errorf("failed to make sample chunk: %w", err)
	}
	rscData := rsc.Data()

	hasher, trHasher := bmtpool.Get(), bmt.NewTrHasher(sampleData.Salt)
	defer bmtpool.Put(hasher)

	ssn := segmentSelection(sampleData.Salt)

	wcp := Trio[int]{} // wcp = witness chunk position
	wcp.Element1 = ssn.Element1 % (sampleLength - 1)
	wcp.Element2 = ssn.Element2 % (sampleLength - 2)
	if wcp.Element2 == wcp.Element1 {
		wcp.Element2 = sampleLength - 2
	}
	wcp.Element3 = sampleLength - 2

	wca := Trio[swarm.Address]{} // wca = witness chunk address
	wca.Element1 = rsi[wcp.Element1].ChunkAddress
	wca.Element2 = rsi[wcp.Element2].ChunkAddress
	wca.Element3 = rsi[wcp.Element3].ChunkAddress

	wc, err := getWitnessChunks(wca)
	if err != nil {
		return proofs, fmt.Errorf("failed getting witness chunks: %w", err)
	}

	wp := Trio[bmt.Proof]{} // witness chunks proof
	wp.Element1 = makeProof(hasher, rscData, 2*wcp.Element1)
	wp.Element2 = makeProof(hasher, rscData, 2*wcp.Element2)
	wp.Element3 = makeProof(hasher, rscData, 2*wcp.Element3)

	rp := Trio[bmt.Proof]{} // rp = retention proofs
	rp.Element1 = makeProof(hasher, wc.Element1.Data(), ssn.Element1%128)
	rp.Element2 = makeProof(hasher, wc.Element2.Data(), ssn.Element2%128)
	rp.Element3 = makeProof(hasher, wc.Element3.Data(), ssn.Element3%128)

	trp := Trio[bmt.Proof]{} // trp = transformed address proofs
	trp.Element1 = makeProof(trHasher, wc.Element1.Data(), ssn.Element1%128)
	trp.Element2 = makeProof(trHasher, wc.Element2.Data(), ssn.Element2%128)
	trp.Element3 = makeProof(trHasher, wc.Element3.Data(), ssn.Element3%128)

	proofs.Element1 = redistribution.ChunkInclusionProof{
		PostageStamp:            wc.Element1.Stamp(),
		WitnessProof:            wp.Element1,
		RetentionProof:          rp.Element1,
		TransformedAddressProof: trp.Element1,
	}
	proofs.Element2 = redistribution.ChunkInclusionProof{
		PostageStamp:            wc.Element2.Stamp(),
		WitnessProof:            wp.Element2,
		RetentionProof:          rp.Element2,
		TransformedAddressProof: trp.Element2,
	}
	proofs.Element3 = redistribution.ChunkInclusionProof{
		PostageStamp:            wc.Element3.Stamp(),
		WitnessProof:            wp.Element3,
		RetentionProof:          rp.Element3,
		TransformedAddressProof: trp.Element3,
	}

	return proofs, nil
}

func makeProof(h *bmt.Hasher, data []byte, j int) bmt.Proof {
	h.Reset()
	_, _ = h.Write(data)
	_, _ = h.Hash(nil)

	p := bmt.Prover{Hasher: h}
	return p.Proof(j)
}

func segmentSelection(salt []byte) Trio[int] {
	// TODO
	// SSN(g,i) = H(R(g)|BE_8(i))

	sl := len(salt)
	be := make([]byte, sl+1)
	copy(be, salt)

	ssn := func(i int) int {
		be[sl] = byte(i)
		return int(binary.BigEndian.Uint32(be))
	}

	return Trio[int]{
		Element1: ssn(0),
		Element2: ssn(1),
		Element3: ssn(2),
	}
}

func getWitnessChunks(wca Trio[swarm.Address]) (Trio[swarm.Chunk], error) {
	// TODO load chunks from store
	return Trio[swarm.Chunk]{}, nil
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
