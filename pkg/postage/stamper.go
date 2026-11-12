// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/crypto"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// ErrBucketFull is the error when a collision bucket is full.
var ErrBucketFull = errors.New("bucket full")

// Stamper can issue stamps from the given address of chunk.
type Stamper interface {
	// addr is the request address of the chunk and idAddr is the identity address of the chunk.
	Stamp(addr, idAddr swarm.Address) (*Stamp, error)
	BatchId() []byte
}

// stamper connects a stampissuer with a signer.
// A stamper is created for each upload session.
type stamper struct {
	store  storage.Store
	issuer *StampIssuer
	signer crypto.Signer
	// generation is the issuer generation the stamper was created in.
	// Once the issuer is reset the stamper refuses to issue stamps, so
	// an upload session never reuses indices it has already issued.
	generation uint32
}

// NewStamper constructs a Stamper.
func NewStamper(store storage.Store, issuer *StampIssuer, signer crypto.Signer) Stamper {
	return &stamper{
		store:      store,
		issuer:     issuer,
		signer:     signer,
		generation: issuer.Generation(),
	}
}

// Stamp takes chunk, see if the chunk can be included in the batch and
// signs it with the owner of the batch of this Stamp issuer.
func (st *stamper) Stamp(addr, idAddr swarm.Address) (*Stamp, error) {
	st.issuer.mtx.Lock()
	defer st.issuer.mtx.Unlock()

	generation := st.issuer.data.Generation
	if generation != st.generation {
		return nil, ErrStampIssuerReset
	}

	item := &StampItem{
		BatchID:      st.issuer.data.BatchID,
		chunkAddress: idAddr,
	}
	err := st.store.Get(item)
	if err == nil && item.Generation != generation {
		// The index was issued before the last reset and may since have
		// been issued to another chunk, so a new one is needed.
		err = storage.ErrNotFound
	}
	switch {
	case err == nil:
		item.BatchTimestamp = unixTime()
		if err = st.store.Put(item); err != nil {
			return nil, err
		}
	case errors.Is(err, storage.ErrNotFound):
		item.Generation = generation
		item.BatchIndex, item.BatchTimestamp, err = st.issuer.increment(addr)
		if err != nil {
			return nil, err
		}
		if err := st.store.Put(item); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("get stamp for %s: %w", item, err)
	}

	toSign, err := ToSignDigest(
		addr.Bytes(),
		st.issuer.data.BatchID,
		item.BatchIndex,
		item.BatchTimestamp,
	)
	if err != nil {
		return nil, err
	}
	sig, err := st.signer.Sign(toSign)
	if err != nil {
		return nil, err
	}
	return NewStamp(st.issuer.data.BatchID, item.BatchIndex, item.BatchTimestamp, sig), nil
}

// BatchId gives back batch id of stamper
func (st *stamper) BatchId() []byte {
	return st.issuer.data.BatchID
}

type presignedStamper struct {
	stamp *Stamp
	owner []byte
}

func NewPresignedStamper(stamp *Stamp, owner []byte) Stamper {
	return &presignedStamper{stamp, owner}
}

func (st *presignedStamper) Stamp(addr, _ swarm.Address) (*Stamp, error) {
	// check stored stamp is against the chunk address
	// Recover the public key from the signature
	signerAddr, err := RecoverBatchOwner(addr, st.stamp)
	if err != nil {
		return nil, err
	}

	if !bytes.Equal(st.owner, signerAddr) {
		return nil, ErrInvalidBatchSignature
	}

	return st.stamp, nil
}

func (st *presignedStamper) BatchId() []byte {
	return st.stamp.BatchID()
}
