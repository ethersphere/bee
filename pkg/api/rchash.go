// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package api

import (
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storageincentives/redistribution"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

type SampleWithProofs struct {
	Hash     swarm.Address `json:"hash"`
	Proofs   []Proof       `json:"proofs"`
	Duration time.Duration `json:"duration"`
}

// Proof structure must exactly match
// corresponding structure (of the same name) in Redistribution.sol smart contract.
// github.com/ethersphere/storage-incentives/blob/ph_f2/src/Redistribution.sol
// github.com/ethersphere/storage-incentives/blob/master/src/Redistribution.sol (when merged to master)
type Proof struct {
	Sisters      []string     `json:"sisters"`
	Data         string       `json:"data"`
	Sisters2     []string     `json:"sisters2"`
	Data2        string       `json:"data2"`
	Sisters3     []string     `json:"sisters3"`
	ChunkSpan    uint64       `json:"chunkSpan"`
	PostageProof PostageProof `json:"postageProof"`
	SocProof     []SOCProof   `json:"socProof"`
}

// SOCProof structure must exactly match
// corresponding structure (of the same name) in Redistribution.sol smart contract.
type PostageProof struct {
	Signature string `json:"signature"`
	BatchID   string `json:"BatchID"`
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

func renderProofs(proofs []redistribution.Proof) []Proof {
	out := make([]Proof, 3)
	for i, p := range proofs {
		out[i] = renderProof(p)
	}
	return out
}

func renderProof(proof redistribution.Proof) Proof {
	var socProof []SOCProof
	if len(proof.SocProof) == 1 {
		socProof = []SOCProof{
			{
				Signer:     toHex(proof.SocProof[0].Signer),
				Signature:  toHex(proof.SocProof[0].Signature),
				Identifier: toHex(proof.SocProof[0].Identifier[:]),
				ChunkAddr:  toHex(proof.SocProof[0].ChunkAddr[:]),
			},
		}
	}

	return Proof{
		Data:      toHex(proof.ProveSegment[:]),
		Sisters:   renderHash(proof.ProofSegments...),
		Data2:     toHex(proof.ProveSegment2[:]),
		Sisters2:  renderHash(proof.ProofSegments2...),
		Sisters3:  renderHash(proof.ProofSegments3...),
		ChunkSpan: proof.ChunkSpan,
		PostageProof: PostageProof{
			Signature: toHex(proof.PostageProof.Signature),
			BatchID:   toHex(proof.PostageProof.BatchId[:]),
			Index:     strconv.FormatUint(proof.PostageProof.Index, 16),
			TimeStamp: strconv.FormatUint(proof.PostageProof.TimeStamp, 16),
		},
		SocProof: socProof,
	}
}

func renderHash(hs ...common.Hash) []string {
	output := make([]string, len(hs))
	for i, h := range hs {
		output[i] = hex.EncodeToString(h.Bytes())
	}
	return output
}

var toHex func([]byte) string = hex.EncodeToString

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

	var round uint64
	swp, err := s.sampler.ReserveSampleWithProofs(r.Context(), anchor1, anchor2, paths.Depth, round)
	if err != nil {
		logger.Error(err, "failed making sample with proofs")
		jsonhttp.InternalServerError(w, "failed making sample with proofs")
		return
	}

	resp := SampleWithProofs{
		Hash:     swp.Hash,
		Duration: swp.Duration,
		Proofs:   renderProofs(swp.Proofs),
	}

	jsonhttp.OK(w, resp)
}
