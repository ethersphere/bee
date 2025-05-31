//go:build js
// +build js

package batchstore

import (
	"errors"
	"math"
	"math/big"
	"sync"
	"sync/atomic"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/storage"
)

// store implements postage.Storer
type store struct {
	capacity int
	store    storage.StateStorer // State store backend to persist batches.

	cs atomic.Pointer[postage.ChainState]

	radius  atomic.Uint32
	evictFn evictFn // evict function
	logger  log.Logger

	batchExpiry postage.BatchExpiryHandler

	mtx sync.RWMutex
}

// New constructs a new postage batch store.
// It initialises both chain state and reserve state from the persistent state store.
func New(st storage.StateStorer, ev evictFn, capacity int, logger log.Logger) (postage.Storer, error) {
	cs := &postage.ChainState{}
	err := st.Get(chainStateKey, cs)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
		cs = &postage.ChainState{
			Block:        0,
			TotalAmount:  big.NewInt(0),
			CurrentPrice: big.NewInt(0),
		}
	}
	var radius uint8
	err = st.Get(reserveRadiusKey, &radius)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
	}

	s := &store{
		capacity: capacity,
		store:    st,
		evictFn:  ev,
		logger:   logger.WithName(loggerName).Register(),
	}
	s.cs.Store(cs)

	s.radius.Store(uint32(radius))

	return s, nil
}

// computeRadius calculates the radius by using the sum of all batch depths
// and the node capacity using the formula totalCommitment/node_capacity = 2^R.
// Must be called under lock.
func (s *store) computeRadius() error {

	var totalCommitment int

	err := s.store.Iterate(batchKeyPrefix, func(key, value []byte) (bool, error) {

		b := &postage.Batch{}
		if err := b.UnmarshalBinary(value); err != nil {
			return false, err
		}

		totalCommitment += exp2(uint(b.Depth))

		return false, nil
	})
	if err != nil {
		return err
	}

	var radius uint8
	if totalCommitment > s.capacity {
		// totalCommitment/node_capacity = 2^R
		// log2(totalCommitment/node_capacity) = R
		radius = uint8(math.Ceil(math.Log2(float64(totalCommitment) / float64(s.capacity))))
	}

	s.radius.Store(uint32(radius))

	return s.store.Put(reserveRadiusKey, &radius)
}
