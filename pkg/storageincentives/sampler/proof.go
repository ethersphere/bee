// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sampler

import (
	"math/big"

	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/storageincentives/redistribution"
)

// MakeProofs creates transaction data for the claim method aka Proof of entitlement (POE).
func MakeProofs(
	sample Data,
	anchor2 []byte,
) ([]redistribution.Proof, error) {
	// witness indexes selecting the index-th item of the sample
	index3 := uint64(Size) - 1
	rand := new(big.Int).SetBytes(anchor2)
	index1 := new(big.Int).Mod(rand, new(big.Int).SetUint64(index3)).Uint64()
	index2 := new(big.Int).Mod(rand, new(big.Int).SetUint64(index3-1)).Uint64()
	if index2 >= index1 {
		index2++
	}

	// reserve sample inclusion proofs for witness chunks
	indexes := []uint64{index1, index2, index3}
	witnessProofs := make([]*bmt.Proof, len(indexes))
	for i, idx := range indexes {
		proof := sample.Prover.Proof(int(idx) * 2)
		witnessProofs[i] = &proof
	}

	// data retention proofs for the witness chunks
	segmentIndex := int(new(big.Int).Mod(new(big.Int).SetBytes(anchor2), big.NewInt(int64(128))).Uint64())
	retentionProofs := make([]*bmt.Proof, len(indexes))
	transformProofs := make([]*bmt.Proof, len(indexes))
	for i, idx := range indexes {
		item := sample.Items[idx]
		proof := item.Prover.Proof(segmentIndex)
		retentionProofs[i] = &proof
		transProof := sample.Items[idx].TransProver.Proof(segmentIndex)
		transformProofs[i] = &transProof
	}

	proofs := make([]redistribution.Proof, len(indexes))
	for i := range proofs {
		proofs[i] = redistribution.NewProof(*witnessProofs[i], *retentionProofs[i], *transformProofs[i], sample.Items[i].Stamp, sample.Items[i].SOC)
	}
	return proofs, nil
}
