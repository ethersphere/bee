// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math/big"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/swarm"
)

// StampSize is the number of bytes in the serialisation of a stamp
const StampSize = 97

var (
	// ErrOwnerMismatch is the error given for invalid signatures.
	ErrOwnerMismatch = errors.New("owner mismatch")
	// ErrStampInvalid is the error given if stamp cannot deserialise.
	ErrStampInvalid = errors.New("invalid stamp")
)

// Batch represents a postage batch, a payment on the blockchain.
type Batch struct {
	ID    []byte   // batch ID
	Value *big.Int // overall balance of the batch
	Start uint64   // blocknumber the batch was created
	Owner []byte   // owner's ethereum address
	Depth uint8    // batch depth, i.e., size = 2^{depth}
}

// MarshalBinary serialises a postage batch to a byte slice len 117.
func (b *Batch) MarshalBinary() ([]byte, error) {
	out := make([]byte, 93)
	copy(out, b.ID)
	value := b.Value.Bytes()
	copy(out[64-len(value):], value)
	binary.BigEndian.PutUint64(out[64:72], b.Start)
	copy(out[72:], b.Owner)
	out[92] = b.Depth
	return out, nil
}

// UnmarshalBinary deserialises the batch.
// Unsafe on slice index (len(buf) = 117) as only internally used in db.
func (b *Batch) UnmarshalBinary(buf []byte) error {
	b.ID = buf[:32]
	b.Value = big.NewInt(0).SetBytes(buf[32:64])
	b.Start = binary.BigEndian.Uint64(buf[64:72])
	b.Owner = buf[72:92]
	b.Depth = buf[92]
	return nil
}

// Valid checks the validity of the postage stamp; in particular:
// - authenticity - check batch is valid on the blockchain
// - authorisation - the batch owner is the stamp signer
// the validity  check is only meaningful in its association of a chunk
// this chunk address needs to be given as argument
func (s *Stamp) Valid(chunkAddr swarm.Address, ownerAddr []byte) error {
	toSign, err := toSignDigest(chunkAddr, s.batchID)
	if err != nil {
		return err
	}
	signerPubkey, err := crypto.Recover(s.sig, toSign)
	if err != nil {
		return err
	}
	signerAddr, err := crypto.NewEthereumAddress(*signerPubkey)
	if err != nil {
		return err
	}
	if !bytes.Equal(signerAddr, ownerAddr) {
		return ErrOwnerMismatch
	}
	return nil
}

var _ swarm.Stamp = (*Stamp)(nil)

// Stamp represents a postage stamp as attached to a chunk.
type Stamp struct {
	batchID []byte // postage batch ID
	sig     []byte // common r[32]s[32]v[1]-style 65 byte ECDSA signature
}

// MewStamp constructs a stamp
func NewStamp(batchID, sig []byte) *Stamp {
	return &Stamp{batchID, sig}
}

// BatchID
func (s *Stamp) BatchID() []byte {
	return s.batchID
}

// Sig
func (s *Stamp) Sig() []byte {
	return s.sig
}

// MarshalBinary gives the byte slice serialisation of a stamp: batchID[32]|Signature[65].
func (s *Stamp) MarshalBinary() ([]byte, error) {
	buf := make([]byte, StampSize)
	copy(buf, s.batchID)
	copy(buf[32:], s.sig)
	return buf, nil
}

// UnmarshalBinary parses a serialised stamp into id and signature.
func (s *Stamp) UnmarshalBinary(buf []byte) error {
	if len(buf) != StampSize {
		return ErrStampInvalid
	}
	s.batchID = buf[:32]
	s.sig = buf[32:]
	return nil
}

// toSignDigest creates a digest to represent the stamp which is to be signed by the owner.
func toSignDigest(addr swarm.Address, id []byte) ([]byte, error) {
	h := swarm.NewHasher()
	_, err := h.Write(addr.Bytes())
	if err != nil {
		return nil, err
	}
	_, err = h.Write(id)
	if err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
