// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package api

import (
	"encoding/hex"
	"net/http"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/v2/pkg/jsonhttp"
	"github.com/ethersphere/bee/v2/pkg/storageincentives/redistribution"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/gorilla/mux"
)

type RCHashResponse struct {
	Hash            swarm.Address        `json:"hash"`
	Proofs          ChunkInclusionProofs `json:"proofs"`
	DurationSeconds float64              `json:"durationSeconds"`
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
		socProof = []SOCProof{{
			Signer:     hex.EncodeToString(proof.SocProof[0].Signer.Bytes()),
			Signature:  hex.EncodeToString(proof.SocProof[0].Signature[:]),
			Identifier: hex.EncodeToString(proof.SocProof[0].Identifier.Bytes()),
			ChunkAddr:  hex.EncodeToString(proof.SocProof[0].ChunkAddr.Bytes()),
		}}
	}

	return ChunkInclusionProof{
		ProveSegment:   hex.EncodeToString(proof.ProveSegment.Bytes()),
		ProofSegments:  renderCommonHash(proof.ProofSegments),
		ProveSegment2:  hex.EncodeToString(proof.ProveSegment2.Bytes()),
		ProofSegments2: renderCommonHash(proof.ProofSegments2),
		ProofSegments3: renderCommonHash(proof.ProofSegments3),
		ChunkSpan:      proof.ChunkSpan,
		PostageProof: PostageProof{
			Signature: hex.EncodeToString(proof.PostageProof.Signature[:]),
			PostageId: hex.EncodeToString(proof.PostageProof.PostageId[:]),
			Index:     strconv.FormatUint(proof.PostageProof.Index, 16),
			TimeStamp: strconv.FormatUint(proof.PostageProof.TimeStamp, 16),
		},
		SocProof: socProof,
	}
}

func renderCommonHash(proofSegments []common.Hash) []string {
	output := make([]string, len(proofSegments))
	for i, s := range proofSegments {
		output[i] = hex.EncodeToString(s.Bytes())
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
		Hash:            swp.Hash,
		DurationSeconds: swp.Duration.Seconds(),
		Proofs:          renderChunkInclusionProofs(swp.Proofs),
	}

	jsonhttp.OK(w, resp)
}
