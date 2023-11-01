// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Used for inclusion proof utilities

package redistribution

import (
	"encoding/binary"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethersphere/bee/pkg/bmt"
	"github.com/ethersphere/bee/pkg/soc"
	"github.com/ethersphere/bee/pkg/swarm"
)

// Proof structure must exactly match
// corresponding structure (of the same name) in Redistribution.sol smart contract.
// github.com/ethersphere/storage-incentives/blob/master/src/Redistribution.sol
type Proof struct {
	ProofSegments  []common.Hash
	ProveSegment   common.Hash
	ProofSegments2 []common.Hash
	ProveSegment2  common.Hash
	ChunkSpan      uint64
	ProofSegments3 []common.Hash
	PostageProof   PostageProof
	SocProof       []SOCProof
}

// SOCProof structure must exactly match
// corresponding structure (of the same name) in Redistribution.sol smart contract.
type PostageProof struct {
	Signature []byte
	PostageId common.Hash
	Index     uint64
	TimeStamp uint64
}

// SOCProof structure must exactly match
// corresponding structure (of the same name) in Redistribution.sol smart contract.
type SOCProof struct {
	Signer     []byte      `json:"signer"`
	Signature  []byte      `json:"signature"`
	Identifier common.Hash `json:"identifier"`
	ChunkAddr  common.Hash `json:"chunkAddr"`
}

func bytes32(bs ...[]byte) []common.Hash {
	bbs := make([]common.Hash, len(bs))
	for i, b := range bs {
		var bb [32]byte
		copy(bb[:], b)
		bbs[i] = common.Hash(bb)
	}
	return bbs
}

// NewProof transforms arguments to abi-compatible Proof object
func NewProof(wp1, wp2, wp3 bmt.Proof, stamp swarm.Stamp, sch *soc.SOC) Proof {
	var socProof []SOCProof
	if sch == nil {
		socProof = []SOCProof{{
			Signer:     sch.OwnerAddress(),
			Signature:  sch.Signature(),
			Identifier: bytes32(sch.ID())[0],
			ChunkAddr:  bytes32(sch.WrappedChunk().Address().Bytes())[0],
		}}
	}

	return Proof{
		ProofSegments:  bytes32(wp1.Sisters...),
		ProveSegment:   bytes32(wp1.Data)[0],
		ProofSegments2: bytes32(wp2.Sisters...),
		ProveSegment2:  bytes32(wp2.Data)[0],
		ProofSegments3: bytes32(wp3.Sisters...),
		ChunkSpan:      binary.LittleEndian.Uint64(wp2.Span[:swarm.SpanSize]), // should be uint64 on the other size; copied from pkg/api/bytes.go
		PostageProof: PostageProof{
			Signature: stamp.Sig(),
			PostageId: bytes32(stamp.BatchID())[0],
			Index:     binary.BigEndian.Uint64(stamp.Index()),
			TimeStamp: binary.BigEndian.Uint64(stamp.Timestamp()),
		},
		SocProof: socProof,
	}
}
