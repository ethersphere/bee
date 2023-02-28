// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/hex"
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
	Stamp1      Stamp
	Stamp2      Stamp
	StampLast   Stamp
	Time        string
}

type Stamp struct {
	batchID   []byte // postage batch ID
	index     []byte // index of the batch
	timestamp []byte // to signal order when assigning the indexes to multiple chunks
	sig       []byte // common r[32]s[32]v[1]-style 65 byte ECDSA signature of batchID|index|address by owner or grantee
}

// NewStamp constructs a new stamp from a given batch ID, index and signatures.
func NewStamp(batchID, index, timestamp, sig []byte) Stamp {
	return Stamp{batchID, index, timestamp, sig}
}

// TODO: Remove this API before next release. This API is kept mainly for testing
// the sampler till the storage incentives agent falls into place. As a result,
// no documentation or tests are added here. This should be removed before next
// breaking release.
func (s *Service) rchasher(w http.ResponseWriter, r *http.Request) {
	logger := s.logger.WithName("get_rchash").Build()

	paths := struct {
		Depth   uint8  `map:"depth" validate:"required"`
		Anchor  string `map:"anchor" validate:"required"`
		Anchor2 string `map:"anchor2" validate:"required"`
	}{}
	if response := s.mapStructure(mux.Vars(r), &paths); response != nil {
		response("invalid path params", logger, w)
		return
	}

	anch, err := hex.DecodeString(paths.Anchor)
	if err != nil {
		logger.Error(err, "invalid hex params")
		jsonhttp.InternalServerError(w, "invalid hex params")
		return
	}
	anch2, err := hex.DecodeString(paths.Anchor2)
	if err != nil {
		logger.Error(err, "invalid hex params")
		jsonhttp.InternalServerError(w, "invalid hex params")
		return
	}

	start := time.Now()
	sample, err := s.storer.ReserveSample(r.Context(), anch, paths.Depth, uint64(start.UnixNano()))
	if err != nil {
		logger.Error(err, "reserve commitment hasher: failed generating sample")
		jsonhttp.InternalServerError(w, "failed generating sample")
		return
	}

	require1 := new(big.Int).Mod(new(big.Int).SetBytes(anch2), big.NewInt(15)).Uint64()
	require2 := new(big.Int).Mod(new(big.Int).SetBytes(anch2), big.NewInt(14)).Uint64()

	if require2 >= require1 {
		require2++
	}

	segment1 := int(new(big.Int).Mod(new(big.Int).SetBytes(anch2), big.NewInt(int64(len(sample.Items[require1].ChunkItem.Data)/32))).Uint64())

	segment2 := int(new(big.Int).Mod(new(big.Int).SetBytes(anch2), big.NewInt(int64(len(sample.Items[require2].ChunkItem.Data)/32))).Uint64())

	segmentLast := int(new(big.Int).Mod(new(big.Int).SetBytes(anch2), big.NewInt(int64(len(sample.Items[15].ChunkItem.Data)/32))).Uint64())

	stamp1 := NewStamp(
		sample.Items[require1].ChunkItem.BatchID,
		sample.Items[require1].ChunkItem.Index,
		sample.Items[require1].ChunkItem.Timestamp,
		sample.Items[require1].ChunkItem.Sig,
	)

	stamp2 := NewStamp(
		sample.Items[require2].ChunkItem.BatchID,
		sample.Items[require2].ChunkItem.Index,
		sample.Items[require2].ChunkItem.Timestamp,
		sample.Items[require2].ChunkItem.Sig,
	)

	stampLast := NewStamp(
		sample.Items[15].ChunkItem.BatchID,
		sample.Items[15].ChunkItem.Index,
		sample.Items[15].ChunkItem.Timestamp,
		sample.Items[15].ChunkItem.Sig,
	)

	const Capacity = 32

	pool := bmt.NewPool(bmt.NewConf(swarm.NewHasher, swarm.BmtBranches, Capacity))
	trpool := bmt.NewTrPool(bmt.NewTrConf(swarm.NewHasher, anch, swarm.BmtBranches, Capacity))

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
		Stamp1:      stamp1,
		Stamp2:      stamp2,
		StampLast:   stampLast,
		Time:        time.Since(start).String(),
	})
}
