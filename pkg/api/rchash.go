// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/jsonhttp"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/gorilla/mux"
)

type rchash struct {
	Sample    storage.Sample `json:"sample"`
	Proof1    entityProof    `json:"proof1"`
	Proof2    entityProof    `json:"proof2"`
	ProofLast entityProof    `json:"proofLast"`
	Time      string         `json:"time"`
}

type hexProof struct {
	ProofSegments []hexByte
	ProveSegment  hexByte
}

type entityProof struct {
	ProofSegments []hexByte `json:"proofSegments"`
	ProveSegment  hexByte   `json:"proveSegment"`
	// _RCspan is known for RC 32*32

	// Inclusion proof of transformed address
	ProofSegments2 []hexByte `json:"proofSegments2"`
	ProveSegment2  hexByte   `json:"proveSegment2"`
	// proveSegmentIndex2 known from deterministic random selection;
	ChunkSpan uint64 `json:"chunkSpan"`
	//
	ProofSegments3 []hexByte `json:"proofSegments3"`
	//  _proveSegment3 known, is equal _proveSegment2
	// proveSegmentIndex3 know, is equal _proveSegmentIndex2;
	// chunkSpan2 is equal to chunkSpan (as the data is the same)

	Signer    hexByte `json:"signer"`
	Signature hexByte `json:"signature"`
	ChunkAddr hexByte `json:"chunkAddr"`
	PostageId hexByte `json:"postageId"`
	Index     hexByte `json:"index"`
	TimeStamp hexByte `json:"timeStamp"`
}

// toSignDigest creates a digest to represent the stamp which is to be signed by
// the owner.
func toSignDigest(addr, batchId, index, timestamp []byte) ([]byte, error) {
	h := swarm.NewHasher()
	_, err := h.Write(addr)
	if err != nil {
		return nil, err
	}
	_, err = h.Write(batchId)
	if err != nil {
		return nil, err
	}
	_, err = h.Write(index)
	if err != nil {
		return nil, err
	}
	_, err = h.Write(timestamp)
	if err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func bytesToHex(proof bmt.Proof) (hexProof, error) {
	var proveSegment hexByte
	proofSegments := make([]hexByte, len(proof.ProofSegments)+1)
	if proof.Index%2 == 0 {
		proofSegments[0] = proof.ProveSegment[swarm.SectionSize:]
		proveSegment = proof.ProveSegment[:swarm.SectionSize]
	} else {
		proofSegments[0] = proof.ProveSegment[:swarm.SectionSize]
		proveSegment = proof.ProveSegment[swarm.SectionSize:]
	}
	for i, sister := range proof.ProofSegments {
		proofSegments[i+1] = sister
	}

	return hexProof{ProveSegment: proveSegment, ProofSegments: proofSegments}, nil
}

func NewProof(proofp1, proofp2 bmt.Proof, proofp3 bmt.Proof, stamp Stamp, chunkAddress []byte) (entityProof, error) {

	//sanity check if proofp1.Span != 32*32 { return nil, errors.New("failed p1 span check") }

	//      if proofp2.Span != proofp3.Span {
	//              return entityProof{}, errors.New("failed p2p3 span check")
	//      }
	//
	//      if proofp2.Section != proofp3.Section || proofp2.Sisters[0] != proofp3.Sisters[0] {
	//              return entityProof{}, errors.New("failed p2p3 data check")
	//      }

	proofp1Hex, _ := bytesToHex(proofp1)
	proofp2Hex, _ := bytesToHex(proofp2)
	proofp3Hex, _ := bytesToHex(proofp3)

	toSign, err := toSignDigest(chunkAddress, stamp.batchID, stamp.index, stamp.timestamp)
	if err != nil {
		return entityProof{}, err
	}
	signerPubkey, err := crypto.Recover(stamp.sig, toSign)
	if err != nil {
		return entityProof{}, err
	}
	batchOwner, err := crypto.NewEthereumAddress(*signerPubkey)
	if err != nil {
		return entityProof{}, err
	}

	return entityProof{
		proofp1Hex.ProofSegments,
		proofp1Hex.ProveSegment,
		proofp2Hex.ProofSegments,
		proofp2Hex.ProveSegment,
		uint64(binary.LittleEndian.Uint64(proofp2.Span[:swarm.SpanSize])), // should be uint64 on the other size; copied from pkg/api/bytes.go
		proofp3Hex.ProofSegments,
		batchOwner,
		stamp.sig,
		chunkAddress,
		stamp.batchID,
		stamp.index,
		stamp.timestamp,
	}, nil
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
	fmt.Printf("Require1 %d\n", require1)

	if require2 >= require1 {
		require2++
	}

	fmt.Printf("Require2 %d\n", require2)
	fmt.Printf("sampleItems %d\n", len(sample.Items))

	segmentIndex := int(new(big.Int).Mod(new(big.Int).SetBytes(anch2), big.NewInt(int64(128))).Uint64())

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
	trpool := bmt.NewPool(bmt.NewConf(swarm.NewTrHasher(anch), swarm.BmtBranches, Capacity))

	rccontent := pool.Get()

	rccontent.SetHeaderInt64(32 * 32)

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

	chunk1Content.SetHeader(sample.Items[require1].ChunkItem.Data[:swarm.SpanSize])
	chunk1ContentPayload := sample.Items[require1].ChunkItem.Data[swarm.SpanSize:]
	_, err = chunk1Content.Write(chunk1ContentPayload)
	if err != nil {
		logger.Error(err, "reserve commitment hasher: failure in proof creation")
		jsonhttp.InternalServerError(w, "failure in proof creation")
		return
	}

	chunk1TrContent.SetHeader(sample.Items[require1].ChunkItem.Data[:swarm.SpanSize])

	_, err = chunk1TrContent.Write(chunk1ContentPayload)
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

	proof1p2 := bmt.Prover{chunk1Content}.Proof(segmentIndex)
	proof1p3 := bmt.Prover{chunk1TrContent}.Proof(segmentIndex)

	pool.Put(chunk1Content)
	trpool.Put(chunk1TrContent)

	chunk2Content := pool.Get()
	chunk2TrContent := trpool.Get()

	chunk2Content.SetHeader(sample.Items[require2].ChunkItem.Data[:swarm.SpanSize])
	chunk2ContentPayload := sample.Items[require2].ChunkItem.Data[swarm.SpanSize:]
	_, err = chunk2Content.Write(chunk2ContentPayload)
	if err != nil {
		logger.Error(err, "reserve commitment hasher: failure in proof creation")
		jsonhttp.InternalServerError(w, "failure in proof creation")
		return
	}

	chunk2TrContent.SetHeader(sample.Items[require2].ChunkItem.Data[:swarm.SpanSize])
	_, err = chunk2TrContent.Write(chunk2ContentPayload)
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

	proof2p2 := bmt.Prover{chunk2Content}.Proof(segmentIndex)
	proof2p3 := bmt.Prover{chunk2TrContent}.Proof(segmentIndex)

	pool.Put(chunk2Content)
	trpool.Put(chunk2TrContent)

	chunkLastContent := pool.Get()
	chunkLastTrContent := trpool.Get()

	chunkLastContent.SetHeader(sample.Items[15].ChunkItem.Data[:swarm.SpanSize])
	chunkLastContentPayload := sample.Items[15].ChunkItem.Data[swarm.SpanSize:]
	_, err = chunkLastContent.Write(chunkLastContentPayload)
	if err != nil {
		logger.Error(err, "reserve commitment hasher: failure in proof creation")
		jsonhttp.InternalServerError(w, "failure in proof creation")
		return
	}

	// NOTE: ChunkItem.Data prefixed with the spansize
	chunkLastTrContent.SetHeader(sample.Items[15].ChunkItem.Data[:swarm.SpanSize])
	_, err = chunkLastTrContent.Write(chunkLastContentPayload)
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

	proofLastp2 := bmt.Prover{chunkLastContent}.Proof(segmentIndex)
	proofLastp3 := bmt.Prover{chunkLastTrContent}.Proof(segmentIndex)

	pool.Put(chunkLastContent)
	trpool.Put(chunkLastTrContent)

	proof1, err := NewProof(proof1p1, proof1p2, proof1p3, stamp1, sample.Items[require1].ChunkItem.Address)
	if err != nil {
		logger.Error(err, "reserve commitment hasher: failure in proof1 conversion")
		jsonhttp.InternalServerError(w, "failure in proof1 conversion")
		return
	}
	proof2, err := NewProof(proof2p1, proof2p2, proof2p3, stamp2, sample.Items[require2].ChunkItem.Address)
	if err != nil {
		logger.Error(err, "reserve commitment hasher: failure in proof2 conversion")
		jsonhttp.InternalServerError(w, "failure in proof2 conversion")
		return
	}
	proofLast, err := NewProof(proofLastp1, proofLastp2, proofLastp3, stampLast, sample.Items[15].ChunkItem.Address)
	if err != nil {
		logger.Error(err, "reserve commitment hasher: failure in proofLast conversion")
		jsonhttp.InternalServerError(w, "failure in proofLast conversion")
		return
	}

	jsonhttp.OK(w, rchash{
		Sample:    sample,
		Proof1:    proof1,
		Proof2:    proof2,
		ProofLast: proofLast,
		Time:      time.Since(start).String(),
	})
}
