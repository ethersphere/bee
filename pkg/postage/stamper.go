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

var (
	// ErrBucketFull is the error when a collision bucket is full.
	ErrBucketFull = errors.New("bucket full")
)

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
}

// NewStamper constructs a Stamper.
func NewStamper(store storage.Store, issuer *StampIssuer, signer crypto.Signer) Stamper {
	return &stamper{store, issuer, signer}
}

// Stamp takes chunk, see if the chunk can be included in the batch and
// signs it with the owner of the batch of this Stamp issuer.
func (st *stamper) Stamp(addr, idAddr swarm.Address) (*Stamp, error) {
	st.issuer.mtx.Lock()
	defer st.issuer.mtx.Unlock()

	item := &StampItem{
		BatchID:      st.issuer.data.BatchID,
		chunkAddress: idAddr,
	}
	switch err := st.store.Get(item); {
	case err == nil:
		item.BatchTimestamp = unixTime()
		if err = st.store.Put(item); err != nil {
			return nil, err
		}
	case errors.Is(err, storage.ErrNotFound):
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
