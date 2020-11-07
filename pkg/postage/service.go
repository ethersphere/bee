// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"bytes"
	"errors"
	"fmt"
	"sync"

	"github.com/ethersphere/bee/pkg/storage"
)

const (
	postagePrefix = "postage"
)

var (
	// ErrNotFound is the error returned when issuer with given batch ID does not exist.
	ErrNotFound = errors.New("not found")
)

// Service handles postage batches
// stores the active batches.
type Service struct {
	lock    sync.Mutex
	store   storage.StateStorer
	chainID int64
	issuers []*StampIssuer
}

// NewService constructs a new Service.
func NewService(store storage.StateStorer, chainID int64) *Service {
	return &Service{
		store:   store,
		chainID: chainID,
	}
}

// Add adds a stamp issuer to the active issuers.
func (ps *Service) Add(st *StampIssuer) {
	ps.lock.Lock()
	defer ps.lock.Unlock()
	ps.issuers = append(ps.issuers, st)
}

// StampIssuers returns the currently active stamp issuers.
func (ps *Service) StampIssuers() []*StampIssuer {
	ps.lock.Lock()
	defer ps.lock.Unlock()
	return ps.issuers
}

// GetStampIssuer finds a stamp issuer by batch ID.
func (ps *Service) GetStampIssuer(batchID []byte) (*StampIssuer, error) {
	ps.lock.Lock()
	defer ps.lock.Unlock()
	for _, st := range ps.issuers {
		if bytes.Equal(batchID, st.batchID) {
			return st, nil
		}
	}
	return nil, ErrNotFound
}

// Load loads all active batches (stamp issuers) from the statestore.
func (ps *Service) Load() error {
	n := 0
	if err := ps.store.Iterate(ps.key(), func(key, _ []byte) (stop bool, err error) {
		n++
		return false, nil
	}); err != nil {
		return err
	}
	for i := 0; i < n; i++ {
		st := &StampIssuer{}
		err := ps.store.Get(ps.key(i), st)
		if err != nil {
			return err
		}
		ps.Add(st)
	}
	return nil
}

// Save saves all the active stamp issuers to statestore.
func (ps *Service) Save() error {
	for i, st := range ps.issuers {
		if err := ps.store.Put(ps.key(i), st); err != nil {
			return err
		}
	}
	return nil
}

// key returns the statestore key for an issuer
// to disambiguate batches on different chains, chainID is part of the key
// if called with no index argument, it returns the common prefix used for iteration
func (ps *Service) key(v ...int) string {
	prefix := fmt.Sprintf(postagePrefix+"%d", ps.chainID)
	if len(v) == 0 {
		return prefix
	}
	return fmt.Sprintf(prefix+"%d", v[0])
}
