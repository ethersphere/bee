// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/storage"
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

// NewStamp constructs a new stamp from a given batch ID and signature.
func NewStamp(batchID, sig []byte) *Stamp {
	return &Stamp{batchID, sig}
}

// BatchID returns the batch ID of the stamp.
func (s *Stamp) BatchID() []byte {
	return s.batchID
}

// Sig returns the signature of the stamp.
func (s *Stamp) Sig() []byte {
	return s.sig
}

// MarshalBinary gives the byte slice serialisation of a stamp:
// batchID[32]|Signature[65].
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

// toSignDigest creates a digest to represent the stamp which is to be signed by
// the owner.
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

// ValidStamp returns a stampvalidator function passed to protocols with chunk entrypoints.
func ValidStamp(batchStore Storer) func(chunk swarm.Chunk, stampBytes []byte) (swarm.Chunk, error) {
	return func(chunk swarm.Chunk, stampBytes []byte) (swarm.Chunk, error) {
		stamp := new(Stamp)
		err := stamp.UnmarshalBinary(stampBytes)
		if err != nil {
			return nil, err
		}
		b, err := batchStore.Get(stamp.BatchID())
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return nil, fmt.Errorf("batchstore get: %v, %w", err, ErrNotFound)
			}
			return nil, err
		}
		if err = stamp.Valid(chunk.Address(), b.Owner); err != nil {
			return nil, fmt.Errorf("chunk %s stamp invalid: %w", chunk.Address().String(), err)
		}
		return chunk.WithStamp(stamp).WithBatch(b.Radius, b.Depth), nil
	}
}
