// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"errors"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	// ErrBucketFull is the error when a collision bucket is full.
	ErrBucketFull = errors.New("bucket full")
)

// Stamper can issue stamps from the given address of chunk.
type Stamper interface {
	Stamp(swarm.Address) (*Stamp, error)
}

// stamper connects a stampissuer with a signer.
// A stamper is created for each upload session.
type stamper struct {
	issuer *StampIssuer
	signer crypto.Signer
}

// NewStamper constructs a Stamper.
func NewStamper(st *StampIssuer, signer crypto.Signer) Stamper {
	return &stamper{st, signer}
}

// Stamp takes chunk, see if the chunk can included in the batch and
// signs it with the owner of the batch of this Stamp issuer.
func (st *stamper) Stamp(addr swarm.Address) (*Stamp, error) {
	batchIndex, batchTimestamp, err := st.issuer.increment(addr)
	if err != nil {
		return nil, err
	}

	toSign, err := toSignDigest(
		addr.Bytes(),
		st.issuer.data.BatchID,
		batchIndex,
		batchTimestamp,
	)
	if err != nil {
		return nil, err
	}
	sig, err := st.signer.Sign(toSign)
	if err != nil {
		return nil, err
	}
	return NewStamp(st.issuer.data.BatchID, batchIndex, batchTimestamp, sig), nil
}
