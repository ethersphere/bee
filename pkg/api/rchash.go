// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"math/big"
	"net/http"
	"time"

	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

type rchash struct {
	Sample      storage.Sample
	Proof1p1    bmt.Proof
	Proof1p2    bmt.Proof
	Proof1p3    bmt.Proof
	Proof2p1    bmt.Proof
	Proof2p2    bmt.Proof
	Proof2p3    bmt.Proof
	ProofLastp1 bmt.Proof
	ProofLastp2 bmt.Proof
	ProofLastp3 bmt.Proof
	Time        string
}

// TODO: Remove this API before next release. This API is kept mainly for testing
// the sampler till the storage incentives agent falls into place. As a result,
// no documentation or tests are added here. This should be removed before next
// breaking release.
func (s *Service) rchasher(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_rchash").Build()

	paths := struct {
		Depth  uint8  `map:"depth" validate:"required"`
		Anchor string `map:"anchor" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	start := time.Now()
	sample, err := s.storer.ReserveSample(r.Context(), []byte(paths.Anchor), paths.Depth, uint64(start.UnixNano()))
	if err != nil {
		logger.Error(err, "reserve commitment hasher: failed generating sample")
		jsonhttp.InternalServerError(w, "failed generating sample")
		return
	}

	require1 := new(big.Int).Mod(new(big.Int).SetBytes([]byte(paths.Anchor)), big.NewInt(15)).Uint64()
	require2 := new(big.Int).Mod(new(big.Int).SetBytes([]byte(paths.Anchor)), big.NewInt(14)).Uint64()

	segment1 := int(new(big.Int).Mod(new(big.Int).SetBytes([]byte(paths.Anchor)), big.NewInt(int64(len(sample.Items[require1].ChunkItem.Data)/32))).Uint64())

	segment2 := int(new(big.Int).Mod(new(big.Int).SetBytes([]byte(paths.Anchor)), big.NewInt(int64(len(sample.Items[require2].ChunkItem.Data)/32))).Uint64())

	segmentLast := int(new(big.Int).Mod(new(big.Int).SetBytes([]byte(paths.Anchor)), big.NewInt(int64(len(sample.Items[15].ChunkItem.Data)/32))).Uint64())

	if require2 >= require1 {
		require2++
	}

	const Capacity = 32

	pool := bmt.NewPool(bmt.NewConf(swarm.NewHasher, swarm.BmtBranches, Capacity))
	trpool := bmt.NewTrPool(bmt.NewTrConf(swarm.NewHasher, []byte(paths.Anchor), swarm.BmtBranches, Capacity))

	rccontent := pool.Get()

	_, err = rccontent.Write(sample.SampleContent)
	if err != nil {
		logger.Error(err, "reserve commitment hasher: failure in proof creation")
		jsonhttp.InternalServerError(w, "failure in proof creation")
		return
	}

	_, err = rccontent.Hash(nil)
	if err != nil {
		logger.Error(err, "reserve commitment hasher: failure in proof creation")
		jsonhttp.InternalServerError(w, "failure in proof creation")
		return
	}

	proof1p1 := bmt.Prover{rccontent}.Proof(int(require1) * 2)
	proof2p1 := bmt.Prover{rccontent}.Proof(int(require2) * 2)
	proofLastp1 := bmt.Prover{rccontent}.Proof(30)

	pool.Put(rccontent)

	chunk1Content := pool.Get()
	chunk1TrContent := trpool.Get()

	_, err = chunk1Content.Write(sample.Items[require1].ChunkItem.Data)
	if err != nil {
		logger.Error(err, "reserve commitment hasher: failure in proof creation")
		jsonhttp.InternalServerError(w, "failure in proof creation")
		return
	}

	_, err = chunk1TrContent.Write(sample.Items[require1].ChunkItem.Data)
	if err != nil {
		logger.Error(err, "reserve commitment hasher: failure in proof creation")
		jsonhttp.InternalServerError(w, "failure in proof creation")
		return
	}

	_, err = chunk1Content.Hash(nil)
	if err != nil {
		logger.Error(err, "reserve commitment hasher: failure in proof creation")
		jsonhttp.InternalServerError(w, "failure in proof creation")
		return
	}

	_, err = chunk1TrContent.Hash(nil)
	if err != nil {
		logger.Error(err, "reserve commitment hasher: failure in proof creation")
		jsonhttp.InternalServerError(w, "failure in proof creation")
		return
	}

	proof1p2 := bmt.Prover{chunk1Content}.Proof(segment1)
	proof1p3 := bmt.TrProver{chunk1TrContent}.Proof(segment1)

	pool.Put(chunk1Content)
	trpool.Put(chunk1TrContent)

	chunk2Content := pool.Get()
	chunk2TrContent := trpool.Get()

	_, err = chunk2Content.Write(sample.Items[require2].ChunkItem.Data)
	if err != nil {
		logger.Error(err, "reserve commitment hasher: failure in proof creation")
		jsonhttp.InternalServerError(w, "failure in proof creation")
		return
	}

	_, err = chunk2TrContent.Write(sample.Items[require2].ChunkItem.Data)
	if err != nil {
		logger.Error(err, "reserve commitment hasher: failure in proof creation")
		jsonhttp.InternalServerError(w, "failure in proof creation")
		return
	}

	_, err = chunk2Content.Hash(nil)
	if err != nil {
		logger.Error(err, "reserve commitment hasher: failure in proof creation")
		jsonhttp.InternalServerError(w, "failure in proof creation")
		return
	}

	_, err = chunk2TrContent.Hash(nil)
	if err != nil {
		logger.Error(err, "reserve commitment hasher: failure in proof creation")
		jsonhttp.InternalServerError(w, "failure in proof creation")
		return
	}

	proof2p2 := bmt.Prover{chunk2Content}.Proof(segment2)
	proof2p3 := bmt.TrProver{chunk2TrContent}.Proof(segment2)

	pool.Put(chunk2Content)
	trpool.Put(chunk2TrContent)

	chunkLastContent := pool.Get()
	chunkLastTrContent := trpool.Get()

	_, err = chunkLastContent.Write(sample.Items[15].ChunkItem.Data)
	if err != nil {
		logger.Error(err, "reserve commitment hasher: failure in proof creation")
		jsonhttp.InternalServerError(w, "failure in proof creation")
		return
	}

	_, err = chunkLastTrContent.Write(sample.Items[15].ChunkItem.Data)
	if err != nil {
		logger.Error(err, "reserve commitment hasher: failure in proof creation")
		jsonhttp.InternalServerError(w, "failure in proof creation")
		return
	}

	_, err = chunkLastContent.Hash(nil)
	if err != nil {
		logger.Error(err, "reserve commitment hasher: failure in proof creation")
		jsonhttp.InternalServerError(w, "failure in proof creation")
		return
	}

	_, err = chunkLastTrContent.Hash(nil)
	if err != nil {
		logger.Error(err, "reserve commitment hasher: failure in proof creation")
		jsonhttp.InternalServerError(w, "failure in proof creation")
		return
	}

	proofLastp2 := bmt.Prover{chunkLastContent}.Proof(int(segmentLast))
	proofLastp3 := bmt.TrProver{chunkLastTrContent}.Proof(int(segmentLast))

	pool.Put(chunkLastContent)
	trpool.Put(chunkLastTrContent)

	jsonhttp.OK(w, rchash{
		Sample:      sample,
		Proof1p1:    proof1p1,
		Proof1p2:    proof1p2,
		Proof1p3:    proof1p3,
		Proof2p1:    proof2p1,
		Proof2p2:    proof2p2,
		Proof2p3:    proof2p3,
		ProofLastp1: proofLastp1,
		ProofLastp2: proofLastp2,
		ProofLastp3: proofLastp3,
		Time:        time.Since(start).String(),
	})
}
