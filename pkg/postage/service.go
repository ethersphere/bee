// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"math/big"
	"sync"

	"github.com/ethersphere/bee/pkg/log"
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
	store        storage.Store
	postageStore Storer
	chainID      int64
	issuers      []*StampIssuer
}

// NewService constructs a new Service.
func NewService(store storage.Store, postageStore Storer, chainID int64) (Service, error) {
	s := &service{
		store:        store,
		postageStore: postageStore,
		chainID:      chainID,
	}
	err := store.Iterate(
		storage.Query{
			Factory: func() storage.Item {
				return new(StampIssuerItem)
			},
		}, func(result storage.Result) (bool, error) {
			issuer := result.Entry.(*StampIssuerItem).Issuer
			_ = s.add(issuer)
			return false, nil
		})
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Add adds a stamp issuer to the active issuers.
func (ps *service) Add(st *StampIssuer) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	if !ps.add(st) {
		return nil
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
func (ps *service) HandleTopUp(batchID []byte, amount *big.Int) {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	for _, v := range ps.issuers {
		if bytes.Equal(v.data.BatchID, batchID) {
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
			if (st.logger == nil) {
				st.logger = log.NewLogger("node").WithName("postage").Register()
			}
			st.logger.Debug("postage.GetStampIssuer", "label", st.data.Label, "batch", hex.EncodeToString(st.data.BatchID))
			return st, func() error {
				return ps.save(st)
			}, nil
		}
	}
	return nil, nil, ErrNotFound
}

// save persists the specified stamp issuer to the stamperstore.
func (ps *service) save(st *StampIssuer) error {
	st.bucketMtx.Lock()
	defer st.bucketMtx.Unlock()
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
	var err error
	for _, issuer := range ps.issuers {
		err = errors.Join(err, ps.save(issuer))
	}
	return err
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
	logger := log.NewLogger("node").WithName("postage").Register()
	for _, issuer := range ps.issuers {
		exists, err := ps.postageStore.Exists(issuer.ID())
		if err != nil {
			logger.Error(err, "set expired: checking if issuer exists", "id", issuer.ID())
			return err
		}
		issuer.SetExpired(!exists)
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
	if (st.logger == nil) {
		st.logger = log.NewLogger("node").WithName("postage").Register()
	}
	st.logger.Debug("postage.add(StampIssuer)", "label", st.data.Label, "batch", hex.EncodeToString(st.data.BatchID))
	ps.issuers = append(ps.issuers, st)
	return true
}
