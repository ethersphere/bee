// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/crypto"
	"github.com/ethersphere/bee/pkg/storage"
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
	store  storage.Store
	issuer *StampIssuer
	signer crypto.Signer
}

// NewStamper constructs a Stamper.
func NewStamper(store storage.Store, issuer *StampIssuer, signer crypto.Signer) Stamper {
	return &stamper{store, issuer, signer}
}

// Stamp takes chunk, see if the chunk can included in the batch and
// signs it with the owner of the batch of this Stamp issuer.
func (st *stamper) Stamp(addr swarm.Address) (*Stamp, error) {
	item := &stampItem{
		batchID:      st.issuer.data.BatchID,
		chunkAddress: addr,
	}
	switch err := st.store.Get(item); {
	case err == nil:
		// check if this index is in the past, it could happen that we encountered
		// some error after assigning this index and did not save the issuer data. In
		// this case we should assign a new index and update it.
		if st.issuer.assigned(item.BatchIndex) {
			break
		}
		fallthrough
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
