// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sync"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/storage/inmemstore"
)

const (
	postagePrefix = "postage"
	// blockThreshold is used to allow threshold no of blocks to be synced before a
	// batch is usable.
	blockThreshold = 10
)

var (
	// ErrNotFound is the error returned when issuer with given batch ID does not exist.
	ErrNotFound = errors.New("not found")
	// ErrNotUsable is the error returned when issuer with given batch ID is not usable.
	ErrNotUsable = errors.New("not usable")
)

// Service is the postage service interface.
type Service interface {
	Add(*StampIssuer) error
	StampIssuers() []*StampIssuer
	GetStampIssuer([]byte) (*StampIssuer, func() error, error)
	IssuerUsable(*StampIssuer) bool
	BatchEventListener
	BatchExpiryHandler
	io.Closer
}

// service handles postage batches
// stores the active batches.
type service struct {
	lock         sync.Mutex
	store        storage.StateStorer
	postageStore Storer
	chainID      int64
	issuers      []*StampIssuer
}

// NewService constructs a new Service.
func NewService(store storage.StateStorer, postageStore Storer, chainID int64) (Service, error) {
	s := &service{
		store:        store,
		postageStore: postageStore,
		chainID:      chainID,
	}
	if err := s.store.Iterate(s.key(), func(_, value []byte) (bool, error) {
		st := &StampIssuer{}
		if err := st.UnmarshalBinary(value); err != nil {
			return false, err
		}
		st.store = inmemstore.New()
		_ = s.add(st)
		return false, nil
	}); err != nil {
		return nil, err
	}

	return s, nil
}

// Add adds a stamp issuer to the active issuers.
func (ps *service) Add(st *StampIssuer) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	if ps.add(st) {
		if err := ps.store.Put(ps.keyForIndex(st.data.BatchID), st); err != nil {
			return err
		}
	}

	return nil
}

// add adds a stamp issuer to the active issuers and returns false if it is already present.
func (ps *service) add(st *StampIssuer) bool {

	for _, v := range ps.issuers {
		if bytes.Equal(st.data.BatchID, v.data.BatchID) {
			return false
		}
	}
	ps.issuers = append(ps.issuers, st)

	return true
}

// HandleCreate implements the BatchEventListener interface. This is fired on receiving
// a batch creation event from the blockchain listener to ensure that if a stamp
// issuer was not created initially, we will create it here.
func (ps *service) HandleCreate(b *Batch, amount *big.Int) error {
	return ps.Add(NewStampIssuer(
		"recovered",
		string(b.Owner),
		b.ID,
		amount,
		b.Depth,
		b.BucketDepth,
		b.Start,
		b.Immutable,
	))
}

// HandleTopUp implements the BatchEventListener interface. This is fired on receiving
// a batch topup event from the blockchain to update stampissuer details
func (ps *service) HandleTopUp(batchID []byte, amount *big.Int) {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	for _, v := range ps.issuers {
		if bytes.Equal(batchID, v.data.BatchID) {
			v.data.BatchAmount.Add(v.data.BatchAmount, amount)
		}
	}
}

func (ps *service) HandleDepthIncrease(batchID []byte, newDepth uint8) {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	for _, v := range ps.issuers {
		if bytes.Equal(batchID, v.data.BatchID) {
			if newDepth > v.data.BatchDepth {
				v.data.BatchDepth = newDepth
			}
			return
		}
	}
}

// StampIssuers returns the currently active stamp issuers.
func (ps *service) StampIssuers() []*StampIssuer {
	ps.lock.Lock()
	defer ps.lock.Unlock()
	return ps.issuers
}

func (ps *service) IssuerUsable(st *StampIssuer) bool {
	cs := ps.postageStore.GetChainState()

	// this checks atleast threshold blocks are seen on the blockchain after
	// the batch creation, before we start using a stamp issuer. The threshold
	// is meant to allow enough time for upstream peers to see the batch and
	// hence validate the stamps issued
	if cs.Block < st.data.BlockNumber || (cs.Block-st.data.BlockNumber) < blockThreshold {
		return false
	}
	return true
}

// GetStampIssuer finds a stamp issuer by batch ID.
func (ps *service) GetStampIssuer(batchID []byte) (*StampIssuer, func() error, error) {
	ps.lock.Lock()
	defer ps.lock.Unlock()
	for _, st := range ps.issuers {
		if bytes.Equal(batchID, st.data.BatchID) {
			if !ps.IssuerUsable(st) {
				return nil, nil, ErrNotUsable
			}
			return st, func() error {
				return ps.save(st)
			}, nil
		}
	}
	return nil, nil, ErrNotFound
}

// save persists the specified stamp issuer to the statestore.
func (ps *service) save(st *StampIssuer) error {
	st.bucketMu.Lock()
	defer st.bucketMu.Unlock()
	if err := ps.store.Put(ps.keyForIndex(st.data.BatchID), st); err != nil {
		return err
	}
	return nil
}

func (ps *service) Close() error {
	ps.lock.Lock()
	defer ps.lock.Unlock()
	var closeErr error
	for _, st := range ps.issuers {
		closeErr = errors.Join(closeErr, ps.save(st))
	}

	return closeErr
}

// keyForIndex returns the statestore key for an issuer
func (ps *service) keyForIndex(batchID []byte) string {
	return ps.key() + string(batchID)
}

// key returns the statestore base key for an issuer
// to disambiguate batches on different chains, chainID is part of the key
func (ps *service) key() string {
	return fmt.Sprintf(postagePrefix+"%d", ps.chainID)
}

// HandleStampExpiry handles stamp expiry for a given id.
func (ps *service) HandleStampExpiry(id []byte) {

	ps.lock.Lock()
	defer ps.lock.Unlock()

	for _, v := range ps.issuers {
		if bytes.Equal(id, v.ID()) {
			v.SetExpired(true)
		}
	}
}

// SetExpired sets expiry for all non-existing batches.
func (ps *service) SetExpired() error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	for _, v := range ps.issuers {
		exists, err := ps.postageStore.Exists(v.ID())
		if err != nil {
			return err
		}
		v.SetExpired(!exists)
	}
	return nil
}
