// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"errors"
	"io"
	"math/big"
	"sync"

	"github.com/ethersphere/bee/pkg/storage"
)

const (
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
	StampIssuers() ([]*StampIssuer, error)
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
	store        storage.Store
	postageStore Storer
	chainID      int64
	issuerLocks  map[string]*sync.Mutex
}

// NewService constructs a new Service.
func NewService(store storage.Store, postageStore Storer, chainID int64) Service {
	return &service{
		store:        store,
		postageStore: postageStore,
		chainID:      chainID,
		issuerLocks:  make(map[string]*sync.Mutex),
	}
}

// Add adds a stamp issuer to stamperstore.
func (ps *service) Add(st *StampIssuer) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	return ps.save(st)
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
func (ps *service) HandleTopUp(batchID []byte, amount *big.Int) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	item := NewStampIssuerItem(batchID)
	if err := ps.store.Get(item); err != nil {
		return err
	}

	item.issuer.data.BatchAmount.Add(item.issuer.data.BatchAmount, amount)
	return ps.save(item.issuer)
}

func (ps *service) HandleDepthIncrease(batchID []byte, newDepth uint8) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	item := NewStampIssuerItem(batchID)
	if err := ps.store.Get(item); err != nil {
		return err
	}

	item.issuer.data.BatchDepth = newDepth
	return ps.save(item.issuer)
}

// StampIssuers returns the currently active stamp issuers.
func (ps *service) StampIssuers() ([]*StampIssuer, error) {
	ps.lock.Lock()
	defer ps.lock.Unlock()
	var issuers []*StampIssuer
	if err := ps.store.Iterate(
		storage.Query{
			Factory: func() storage.Item { return new(stampIssuerItem) },
		}, func(result storage.Result) (bool, error) {
			issuers = append(issuers, result.Entry.(*stampIssuerItem).issuer)
			return false, nil
		}); err != nil {
		return nil, err
	}
	return issuers, nil
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

	if _, ok := ps.issuerLocks[string(batchID)]; !ok {
		ps.issuerLocks[string(batchID)] = &sync.Mutex{}
	}
	ps.issuerLocks[string(batchID)].Lock()

	item := NewStampIssuerItem(batchID)
	err := ps.store.Get(item)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}

	if !ps.IssuerUsable(item.issuer) {
		return nil, nil, ErrNotUsable
	}
	return item.issuer, func() error {
		defer ps.issuerLocks[string(batchID)].Unlock()
		return ps.save(item.issuer)
	}, nil
}

// save persists the specified stamp issuer to the stamperstore.
func (ps *service) save(st *StampIssuer) error {
	if err := ps.store.Put(&stampIssuerItem{
		issuer: st,
	}); err != nil {
		return err
	}
	return nil
}

func (ps *service) Close() error {
	ps.lock.Lock()
	defer ps.lock.Unlock()
	return ps.store.Close()
}

// HandleStampExpiry handles stamp expiry for a given id.
func (ps *service) HandleStampExpiry(id []byte) error {

	ps.lock.Lock()
	defer ps.lock.Unlock()

	item := NewStampIssuerItem(id)
	err := ps.store.Get(item)
	if err != nil {
		return err
	}

	item.issuer.SetExpired(true)
	return ps.save(item.issuer)
}

// SetExpired sets expiry for all non-existing batches.
func (ps *service) SetExpired() error {
	ps.lock.Lock()
	defer ps.lock.Unlock()
	return ps.store.Iterate(
		storage.Query{
			Factory: func() storage.Item { return new(stampIssuerItem) },
		}, func(result storage.Result) (bool, error) {
			issuer := result.Entry.(*stampIssuerItem).issuer
			exists, err := ps.postageStore.Exists(issuer.ID())
			if err != nil {
				return true, err
			}
			issuer.SetExpired(!exists)
			err = ps.save(issuer)
			if err != nil {
				return true, err
			}
			return false, nil
		})
}
