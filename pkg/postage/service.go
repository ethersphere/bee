// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sync"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/storage"
)

// loggerName is the tree path name of the logger for this package.
const loggerName = "postage"

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
	logger       log.Logger
	mtx          sync.Mutex
	store        storage.Store
	postageStore Storer
	chainID      int64
	issuers      []*StampIssuer
}

// NewService constructs a new Service.
func NewService(logger log.Logger, store storage.Store, postageStore Storer, chainID int64) (Service, error) {
	s := &service{
		logger:       logger.WithName(loggerName).Register(),
		store:        store,
		postageStore: postageStore,
		chainID:      chainID,
	}

	return s, s.store.Iterate(
		storage.Query{
			Factory: func() storage.Item {
				return new(StampIssuerItem)
			},
		}, func(result storage.Result) (bool, error) {
			issuer := result.Entry.(*StampIssuerItem).Issuer
			_ = s.add(issuer)
			return false, nil
		})
}

// Add adds a stamp issuer to the active issuers.
func (ps *service) Add(st *StampIssuer) error {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

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
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	for _, v := range ps.issuers {
		if bytes.Equal(v.data.BatchID, batchID) {
			v.data.BatchAmount.Add(v.data.BatchAmount, amount)
			return
		}
	}
}

func (ps *service) HandleDepthIncrease(batchID []byte, newDepth uint8) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

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
	ps.mtx.Lock()
	defer ps.mtx.Unlock()
	return ps.issuers
}

func (ps *service) IssuerUsable(st *StampIssuer) bool {
	cs := ps.postageStore.GetChainState()

	// this checks at least threshold blocks are seen on the blockchain after
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
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	for _, st := range ps.issuers {
		if bytes.Equal(batchID, st.data.BatchID) {
			if !ps.IssuerUsable(st) {
				return nil, nil, ErrNotUsable
			}
			return st, func() error {
				ps.mtx.Lock()
				defer ps.mtx.Unlock()
				return ps.save(st)
			}, nil
		}
	}
	return nil, nil, ErrNotFound
}

// save persists the specified stamp issuer to the stamperstore.
func (ps *service) save(st *StampIssuer) error {
	st.mtx.Lock()
	defer st.mtx.Unlock()

	if err := ps.store.Put(&StampIssuerItem{
		Issuer: st,
	}); err != nil {
		return err
	}
	return nil
}

func (ps *service) Close() error {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()
	var err error
	for _, issuer := range ps.issuers {
		err = errors.Join(err, ps.save(issuer))
	}
	return err
}

// HandleStampExpiry handles stamp expiry for a given id.
func (ps *service) HandleStampExpiry(ctx context.Context, id []byte) error {

	exists, err := ps.removeIssuer(id)
	if err != nil {
		return err
	}

	if exists {
		return ps.removeStampItems(ctx, id)
	}

	return nil
}

// removeStampItems
func (ps *service) removeStampItems(ctx context.Context, batchID []byte) error {

	ps.logger.Debug("removing expired stamp items", "batchID", hex.EncodeToString(batchID))

	deleteItemC := make(chan *StampItem)
	go func() {
		for item := range deleteItemC {
			_ = ps.store.Delete(item)
		}
	}()

	count := 0

	defer func() {
		close(deleteItemC)
		ps.logger.Debug("removed expired stamps", "batchID", hex.EncodeToString(batchID), "count", count)
	}()

	return ps.store.Iterate(
		storage.Query{
			Factory: func() storage.Item { return new(StampItem) },
			Prefix:  string(batchID),
		}, func(result storage.Result) (bool, error) {
			select {
			case deleteItemC <- result.Entry.(*StampItem):
			case <-ctx.Done():
				return false, ctx.Err()
			}
			count++
			return false, nil
		})
}

// SetExpired removes all expired batches from the stamp issuers.
func (ps *service) removeIssuer(batchID []byte) (bool, error) {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	for i, issuer := range ps.issuers {
		if bytes.Equal(batchID, issuer.data.BatchID) {
			if err := ps.store.Delete(&StampIssuerItem{Issuer: issuer}); err != nil {
				return true, fmt.Errorf("set expired: delete stamp data for batch %s: %w", hex.EncodeToString(issuer.ID()), err)
			}
			ps.issuers = append(ps.issuers[:i], ps.issuers[i+1:]...)
			return true, nil
		}
	}

	return false, nil
}

// add adds a stamp issuer to the active issuers and returns false if it is already present.
// Must be mutex locked before usage.
func (ps *service) add(st *StampIssuer) bool {
	for _, v := range ps.issuers {
		if bytes.Equal(st.data.BatchID, v.data.BatchID) {
			return false
		}
	}
	ps.issuers = append(ps.issuers, st)
	return true
}
