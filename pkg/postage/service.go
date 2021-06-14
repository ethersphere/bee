// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/ethersphere/bee/pkg/storage"
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
	Add(*StampIssuer)
	StampIssuers() []*StampIssuer
	GetStampIssuer([]byte) (*StampIssuer, error)
	IssuerUsable(*StampIssuer) bool
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

	n := 0
	if err := s.store.Iterate(s.key(), func(_, _ []byte) (stop bool, err error) {
		n++
		return false, nil
	}); err != nil {
		return nil, err
	}
	for i := 0; i < n; i++ {
		st := &StampIssuer{}
		err := s.store.Get(s.keyForIndex(i), st)
		if err != nil {
			return nil, err
		}
		s.Add(st)
	}
	return s, nil
}

// Add adds a stamp issuer to the active issuers.
func (ps *service) Add(st *StampIssuer) {
	ps.lock.Lock()
	defer ps.lock.Unlock()
	ps.issuers = append(ps.issuers, st)
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
	if cs.Block < st.blockNumber || (cs.Block-st.blockNumber) < blockThreshold {
		return false
	}
	return true
}

// GetStampIssuer finds a stamp issuer by batch ID.
func (ps *service) GetStampIssuer(batchID []byte) (*StampIssuer, error) {
	ps.lock.Lock()
	defer ps.lock.Unlock()
	for _, st := range ps.issuers {
		if bytes.Equal(batchID, st.batchID) {
			if !ps.IssuerUsable(st) {
				return nil, ErrNotUsable
			}
			return st, nil
		}
	}
	return nil, ErrNotFound
}

// Close saves all the active stamp issuers to statestore.
func (ps *service) Close() error {
	for i, st := range ps.issuers {
		if err := ps.store.Put(ps.keyForIndex(i), st); err != nil {
			return err
		}
	}
	return nil
}

// keyForIndex returns the statestore key for an issuer
func (ps *service) keyForIndex(i int) string {
	return fmt.Sprintf(ps.key()+"%d", i)
}

// key returns the statestore base key for an issuer
// to disambiguate batches on different chains, chainID is part of the key
func (ps *service) key() string {
	return fmt.Sprintf(postagePrefix+"%d", ps.chainID)
}
