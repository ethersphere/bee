// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"encoding/binary"
	"errors"
	"time"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	// ErrBucketFull is the error when a collision bucket is full.
	ErrBucketFull = errors.New("bucket full")
)

// Stamper can issue stamps from the given address.
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
	index, err := st.issuer.inc(addr)
	if err != nil {
		return nil, err
	}
	ts := timestamp()
	toSign, err := toSignDigest(addr.Bytes(), st.issuer.batchID, index, ts)
	if err != nil {
		return nil, err
	}
	sig, err := st.signer.Sign(toSign)
	if err != nil {
		return nil, err
	}
	return NewStamp(st.issuer.batchID, index, ts, sig), nil
}

func timestamp() []byte {
	ts := make([]byte, 8)
	binary.BigEndian.PutUint64(ts, uint64(time.Now().UnixNano()))
	return ts
}
