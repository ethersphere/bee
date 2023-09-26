// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package api

import (
	"net/http"
	"time"

	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storageincentives/redistribution"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

type RCHashResponse struct {
	Hash     swarm.Address        `json:"hash"`
	Proofs   ChunkInclusionProofs `json:"proofs"`
	Duration time.Duration        `json:"duration"`
}

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
	ProofSegments  []string     `json:"proofSegments"`
	ProveSegment   string       `json:"proveSegment"`
	ProofSegments2 []string     `json:"proofSegments2"`
	ProveSegment2  string       `json:"proveSegment2"`
	ChunkSpan      uint64       `json:"chunkSpan"`
	ProofSegments3 []string     `json:"proofSegments3"`
	PostageProof   PostageProof `json:"postageProof"`
	SocProof       []SOCProof   `json:"socProof"`
}

// SOCProof structure must exactly match
// corresponding structure (of the same name) in Redistribution.sol smart contract.
type PostageProof struct {
	Signature string `json:"signature"`
	PostageId string `json:"postageId"`
	Index     string `json:"index"`
	TimeStamp string `json:"timeStamp"`
}

// SOCProof structure must exactly match
// corresponding structure (of the same name) in Redistribution.sol smart contract.
type SOCProof struct {
	Signer     string `json:"signer"`
	Signature  string `json:"signature"`
	Identifier string `json:"identifier"`
	ChunkAddr  string `json:"chunkAddr"`
}

func renderChunkInclusionProofs(proofs redistribution.ChunkInclusionProofs) ChunkInclusionProofs {
	return ChunkInclusionProofs{
		A: renderChunkInclusionProof(proofs.A),
		B: renderChunkInclusionProof(proofs.B),
		C: renderChunkInclusionProof(proofs.C),
	}
}

func renderChunkInclusionProof(proof redistribution.ChunkInclusionProof) ChunkInclusionProof {
	var socProof []SOCProof
	if len(proof.SocProof) == 1 {
		socProof = []SOCProof{
			SOCProof{
				Signer:     toHexString(proof.SocProof[0].Signer)[0],
				Signature:  toHexString(proof.SocProof[0].Signature)[0],
				Identifier: toHexString(proof.SocProof[0].Identifier)[0],
				ChunkAddr:  toHexString(proof.SocProof[0].ChunkAddr)[0],
			},
		}
	}

	return ChunkInclusionProof{
		ProveSegment:   toHexString(proof.ProveSegment)[0],
		ProofSegments:  toHexString(proof.ProofSegments...),
		ProveSegment2:  toHexString(proof.ProveSegment2)[0],
		ProofSegments2: toHexString(proof.ProofSegments2...),
		ProofSegments3: toHexString(proof.ProofSegments3...),
		ChunkSpan:      proof.ChunkSpan,
		PostageProof: PostageProof{
			Signature: toHexString(proof.PostageProof.Signature)[0],
			PostageId: toHexString(proof.PostageProof.PostageId)[0],
			Index:     toHexString(proof.PostageProof.Index)[0],
			TimeStamp: toHexString(proof.PostageProof.TimeStamp)[0],
		},
		SocProof: socProof,
	}
}

func toHexString(proofSegments ...[]byte) []string {
	output := make([]string, len(proofSegments))
	for i, s := range proofSegments {
		output[i] = hex.EncodeToString(s)
	}
	return output
}

// This API is kept for testing the sampler. As a result, no documentation or tests are added here.
func (s *Service) rchash(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_rchash").Build()

	paths := struct {
		Depth   uint8  `map:"depth"`
		Anchor1 string `map:"anchor1,decHex" validate:"required"`
		Anchor2 string `map:"anchor2,decHex" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	anchor1 := []byte(paths.Anchor1)

	anchor2 := []byte(paths.Anchor2)

	swp, err := s.redistributionAgent.SampleWithProofs(r.Context(), anchor1, anchor2, paths.Depth)
	if err != nil {
		logger.Error(err, "failed making sample with proofs")
		jsonhttp.InternalServerError(w, "failed making sample with proofs")
		return
	}

	resp := RCHashResponse{
		Hash:     swp.Hash,
		Duration: swp.Duration,
		Proofs:   renderChunkInclusionProofs(swp.Proofs),
	}

	jsonhttp.OK(w, RCHashResponse(resp))
}
