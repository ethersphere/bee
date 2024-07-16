// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// StampSize is the number of bytes in the serialisation of a stamp
const (
	StampSize   = 113
	IndexSize   = 8
	BucketDepth = 16
)

var (
	// ErrOwnerMismatch is the error given for invalid signatures.
	ErrOwnerMismatch = errors.New("owner mismatch")
	// ErrInvalidIndex the error given for invalid stamp index.
	ErrInvalidIndex = errors.New("invalid index")
	// ErrStampInvalid is the error given if stamp cannot deserialise.
	ErrStampInvalid = errors.New("invalid stamp")
	// ErrBucketMismatch is the error given if stamp index bucket verification fails.
	ErrBucketMismatch = errors.New("bucket mismatch")
	// ErrInvalidBatchID is the error returned if the batch ID is incorrect
	ErrInvalidBatchID = errors.New("invalid batch ID")
	// ErrInvalidBatchIndex is the error returned if the batch index is incorrect
	ErrInvalidBatchIndex = errors.New("invalid batch index")
	// ErrInvalidBatchTimestamp is the error returned if the batch timestamp is incorrect
	ErrInvalidBatchTimestamp = errors.New("invalid batch timestamp")
	// ErrInvalidBatchSignature is the error returned if the batch signature is incorrect
	ErrInvalidBatchSignature = errors.New("invalid batch signature")
)

var _ swarm.Stamp = (*Stamp)(nil)

// Stamp represents a postage stamp as attached to a chunk.
type Stamp struct {
	batchID   []byte // postage batch ID
	index     []byte // index of the batch
	timestamp []byte // to signal order when assigning the indexes to multiple chunks
	sig       []byte // common r[32]s[32]v[1]-style 65 byte ECDSA signature of batchID|index|address by owner or grantee
}

// NewStamp constructs a new stamp from a given batch ID, index and signatures.
func NewStamp(batchID, index, timestamp, sig []byte) *Stamp {
	return &Stamp{batchID, index, timestamp, sig}
}

// BatchID returns the batch ID of the stamp.
func (s *Stamp) BatchID() []byte {
	return s.batchID
}

// Index returns the within-batch index of the stamp.
func (s *Stamp) Index() []byte {
	return s.index
}

// Sig returns the signature of the stamp by the user
func (s *Stamp) Sig() []byte {
	return s.sig
}

// Timestamp returns the timestamp of the stamp
func (s *Stamp) Timestamp() []byte {
	return s.timestamp
}

func (s *Stamp) Clone() swarm.Stamp {
	if s == nil {
		return nil
	}
	return &Stamp{
		batchID:   append([]byte(nil), s.batchID...),
		index:     append([]byte(nil), s.index...),
		timestamp: append([]byte(nil), s.timestamp...),
		sig:       append([]byte(nil), s.sig...),
	}
}

// Hash returns the hash of the stamp.
func (s *Stamp) Hash() ([]byte, error) {
	hasher := swarm.NewHasher()
	b, err := s.MarshalBinary()
	if err != nil {
		return nil, err
	}
	_, err = hasher.Write(b)
	if err != nil {
		return nil, err
	}
	return hasher.Sum(nil), nil
}

// MarshalBinary gives the byte slice serialisation of a stamp:
// batchID[32]|index[8]|timestamp[8]|Signature[65].
func (s *Stamp) MarshalBinary() ([]byte, error) {
	buf := make([]byte, StampSize)
	if n := copy(buf, s.batchID); n != 32 {
		return nil, ErrInvalidBatchID
	}
	if n := copy(buf[32:40], s.index); n != 8 {
		return nil, ErrInvalidBatchIndex
	}
	if n := copy(buf[40:48], s.timestamp); n != 8 {
		return nil, ErrInvalidBatchTimestamp
	}
	if n := copy(buf[48:], s.sig); n != 65 {
		return nil, ErrInvalidBatchSignature
	}
	return buf, nil
}

// UnmarshalBinary parses a serialised stamp into id and signature.
func (s *Stamp) UnmarshalBinary(buf []byte) error {
	if len(buf) != StampSize {
		return ErrStampInvalid
	}
	s.batchID = buf[:32]
	s.index = buf[32:40]
	s.timestamp = buf[40:48]
	s.sig = buf[48:]
	return nil
}

type stampJson struct {
	BatchID   []byte `json:"batchID"`
	Index     []byte `json:"index"`
	Timestamp []byte `json:"timestamp"`
	Sig       []byte `json:"sig"`
}

func (s *Stamp) MarshalJSON() ([]byte, error) {
	return json.Marshal(&stampJson{
		s.batchID,
		s.index,
		s.timestamp,
		s.sig,
	})
}

func (a *Stamp) UnmarshalJSON(b []byte) error {
	v := &stampJson{}
	err := json.Unmarshal(b, v)
	if err != nil {
		return err
	}
	a.batchID = v.BatchID
	a.index = v.Index
	a.timestamp = v.Timestamp
	a.sig = v.Sig
	return nil
}

// ToSignDigest creates a digest to represent the stamp which is to be signed by the owner.
func ToSignDigest(addr, batchId, index, timestamp []byte) ([]byte, error) {
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

type ValidStampFn func(chunk swarm.Chunk) (swarm.Chunk, error)

// ValidStamp returns a stampvalidator function passed to protocols with chunk entrypoints.
func ValidStamp(batchStore Storer) ValidStampFn {
	return func(chunk swarm.Chunk) (swarm.Chunk, error) {
		stamp := chunk.Stamp()
		b, err := batchStore.Get(stamp.BatchID())
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return nil, fmt.Errorf("batchstore get: %w, %w", err, ErrNotFound)
			}
			return nil, err
		}

		if err = NewStamp(stamp.BatchID(), stamp.Index(), stamp.Timestamp(), stamp.Sig()).Valid(chunk.Address(), b.Owner, b.Depth, b.BucketDepth, b.Immutable); err != nil {
			return nil, err
		}
		return chunk.WithStamp(stamp).WithBatch(b.Depth, b.BucketDepth, b.Immutable), nil
	}
}

// Valid checks the validity of the postage stamp; in particular:
// - authenticity - check batch is valid on the blockchain
// - authorisation - the batch owner is the stamp signer
// the validity  check is only meaningful in its association of a chunk
// this chunk address needs to be given as argument
func (s *Stamp) Valid(chunkAddr swarm.Address, ownerAddr []byte, depth, bucketDepth uint8, immutable bool) error {
	signerAddr, err := RecoverBatchOwner(chunkAddr, s)
	if err != nil {
		return err
	}
	bucket, index := BucketIndexFromBytes(s.index)
	if toBucket(bucketDepth, chunkAddr) != bucket {
		return ErrBucketMismatch
	}
	if index >= 1<<int(depth-bucketDepth) {
		return ErrInvalidIndex
	}
	if !bytes.Equal(signerAddr, ownerAddr) {
		return ErrOwnerMismatch
	}
	return nil
}

// RecoverBatchOwner returns ethereum address that signed postage batch of supplied stamp.
func RecoverBatchOwner(chunkAddr swarm.Address, stamp swarm.Stamp) ([]byte, error) {
	toSign, err := ToSignDigest(chunkAddr.Bytes(), stamp.BatchID(), stamp.Index(), stamp.Timestamp())
	if err != nil {
		return nil, err
	}
	signerPubkey, err := crypto.Recover(stamp.Sig(), toSign)
	if err != nil {
		return nil, err
	}

	return crypto.NewEthereumAddress(*signerPubkey)
}
