// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"io"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// EventUpdater interface definitions reflect the updates triggered by events
// emitted by the postage contract on the blockchain.
type EventUpdater interface {
	Create(id []byte, owner []byte, totalAmount, normalisedBalance *big.Int, depth, bucketDepth uint8, immutable bool, txHash common.Hash) error
	TopUp(id []byte, topUpAmount, normalisedBalance *big.Int, txHash common.Hash) error
	UpdateDepth(id []byte, depth uint8, normalisedBalance *big.Int, txHash common.Hash) error
	UpdatePrice(price *big.Int, txHash common.Hash) error
	UpdateBlockNumber(blockNumber uint64) error
	Start(startBlock uint64, initState *ChainSnapshot, interrupt chan struct{}) error

	TransactionStart() error
	TransactionEnd() error
}

// ChainSnapshot represents the snapshot of all the postage events between the
// FirstBlockNumber and LastBlockNumber. The timestamp stores the time at which the
// snapshot was generated. This snapshot can be used to sync the postage package
// to prevent large no. of chain backend calls.
type ChainSnapshot struct {
	Events           []types.Log `json:"events"`
	LastBlockNumber  uint64      `json:"lastBlockNumber"`
	FirstBlockNumber uint64      `json:"firstBlockNumber"`
	Timestamp        int64       `json:"timestamp"`
}

// UnreserveIteratorFn is used as a callback on Storer.Unreserve method calls.
type UnreserveIteratorFn func(id []byte, radius uint8) (bool, error)

type ReserveStateGetter interface {
	// GetReserveState returns a copy of stored reserve state.
	GetReserveState() *ReserveState
}

// Storer represents the persistence layer for batches
// on the current (highest available) block.
type Storer interface {
	ReserveStateGetter

	// Get returns a batch from the store with the given ID.
	Get([]byte) (*Batch, error)

	// Exists reports whether batch referenced by the give id exists.
	Exists([]byte) (bool, error)

	// Iterate iterates through stored batches.
	Iterate(func(*Batch) (bool, error)) error

	// Save stores given batch in the store. The call is idempotent, so
	// a subsequent call would not create new batches if a batch with
	// such ID already exists.
	Save(*Batch) error

	// Update updates a given batch in the store by first deleting the
	// existing batch and then creating a new one. It's an error to update
	// non-existing batch.
	Update(*Batch, *big.Int, uint8) error

	// GetChainState returns the stored chain state from the store.
	GetChainState() *ChainState

	// PutChainState puts given chain state into the store.
	PutChainState(*ChainState) error

	// SetStorageRadius updates the value of the storage radius atomically.
	SetStorageRadius(func(uint8) uint8) error

	// SetStorageRadiusSetter sets the RadiusSetter to the given value.
	// The given RadiusSetter will be called when radius changes.
	SetStorageRadiusSetter(StorageRadiusSetter)

	// Unreserve evict batches from the unreserve queue of the storage.
	// During the eviction process, the given UnreserveIteratorFn is called.
	Unreserve(UnreserveIteratorFn) error

	// Reset resets chain state and reserve state of the storage.
	Reset() error

	SetBatchExpiryHandler(BatchExpiryHandler)
}

// StorageRadiusSetter is used as a callback when the radius of a node changes.
type StorageRadiusSetter interface {
	SetStorageRadius(uint8)
}

// Listener provides a blockchain event iterator.
type Listener interface {
	io.Closer
	Listen(from uint64, updater EventUpdater, initState *ChainSnapshot) <-chan error
}

type BatchEventListener interface {
	HandleCreate(*Batch, *big.Int) error
	HandleTopUp(id []byte, newBalance *big.Int)
	HandleDepthIncrease(id []byte, newDepth uint8)
}

type BatchExpiryHandler interface {
	HandleStampExpiry([]byte)
	SetExpired() error
}
