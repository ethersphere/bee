// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"context"
	"errors"
	"io"
	"math/big"
	"sync"

	"github.com/ethersphere/bee/pkg/log"
	"github.com/ethersphere/bee/pkg/storage"
	"golang.org/x/sync/semaphore"
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
	GetStampIssuer(context.Context, []byte) (*StampIssuer, func(bool) error, error)
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
	issuersInUse map[string]*semaphore.Weighted
}

// NewService constructs a new Service.
func NewService(store storage.Store, postageStore Storer, chainID int64) Service {
	return &service{
		store:        store,
		postageStore: postageStore,
		chainID:      chainID,
		issuersInUse: make(map[string]*semaphore.Weighted),
	}
}

// Add adds a stamp issuer to stamperstore.
func (ps *service) Add(st *StampIssuer) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	err := ps.store.Get(NewStampIssuerItem(st.ID()))
	if err == nil {
		return nil
	}

	if !errors.Is(err, storage.ErrNotFound) {
		return err
	}
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

	item.Issuer.data.BatchAmount.Add(item.Issuer.data.BatchAmount, amount)
	return ps.save(item.Issuer)
}

func (ps *service) HandleDepthIncrease(batchID []byte, newDepth uint8) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	item := NewStampIssuerItem(batchID)
	if err := ps.store.Get(item); err != nil {
		return err
	}

	item.Issuer.data.BatchDepth = newDepth
	return ps.save(item.Issuer)
}

// StampIssuers returns the currently active stamp issuers.
func (ps *service) StampIssuers() ([]*StampIssuer, error) {
	ps.lock.Lock()
	defer ps.lock.Unlock()
	var issuers []*StampIssuer
	if err := ps.store.Iterate(
		storage.Query{
			Factory: func() storage.Item { return new(StampIssuerItem) },
		}, func(result storage.Result) (bool, error) {
			issuers = append(issuers, result.Entry.(*StampIssuerItem).Issuer)
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
func (ps *service) GetStampIssuer(ctx context.Context, batchID []byte) (*StampIssuer, func(bool) error, error) {

	var issuerAccess *semaphore.Weighted

	ps.lock.Lock()
	if s, ok := ps.issuersInUse[string(batchID)]; ok {
		issuerAccess = s
	} else {
		issuerAccess = semaphore.NewWeighted(1)
		ps.issuersInUse[string(batchID)] = issuerAccess
	}
	ps.lock.Unlock()

	if err := issuerAccess.Acquire(ctx, 1); err != nil {
		return nil, nil, err
	}

	item := NewStampIssuerItem(batchID)
	err := ps.store.Get(item)
	if errors.Is(err, storage.ErrNotFound) {
		issuerAccess.Release(1)
		return nil, nil, ErrNotFound
	}
	if err != nil {
		issuerAccess.Release(1)
		return nil, nil, err
	}
	if !ps.IssuerUsable(item.Issuer) {
		issuerAccess.Release(1)
		return nil, nil, ErrNotUsable
	}

	return item.Issuer, func(update bool) error {
		defer issuerAccess.Release(1)
		if !update {
			return nil
		}
		return ps.save(item.Issuer)
	}, nil
}

// save persists the specified stamp issuer to the stamperstore.
func (ps *service) save(st *StampIssuer) error {
	if err := ps.store.Put(&StampIssuerItem{
		Issuer: st,
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
		if errors.Is(err, storage.ErrNotFound) {
			// HandleStampExpiry is fired for every expiring batch. The postage service
			// only cares about the batches owned by the node, so we can safely ignore
			// batches that are not found in the store.
			return nil
		}
		return err
	}

	item.Issuer.SetExpired(true)
	return ps.save(item.Issuer)
}

// SetExpired sets expiry for all non-existing batches.
func (ps *service) SetExpired() error {
	ps.lock.Lock()
	defer ps.lock.Unlock()
	logger := log.NewLogger("node").WithName("postage").Register()
	return ps.store.Iterate(
		storage.Query{
			Factory: func() storage.Item { return new(StampIssuerItem) },
		}, func(result storage.Result) (bool, error) {
			issuer := result.Entry.(*StampIssuerItem).Issuer
			exists, err := ps.postageStore.Exists(issuer.ID())
			if err != nil {
				logger.Error(err, "set expired: checking if issuer exists", "id", issuer.ID())
				return false, nil
			}
			issuer.SetExpired(!exists)
			err = ps.save(issuer)
			if err != nil {
				return true, err
			}
			return false, nil
		})
}
