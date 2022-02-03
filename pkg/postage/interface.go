// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"io"
	"math/big"
)

// EventUpdater interface definitions reflect the updates triggered by events
// emitted by the postage contract on the blockchain.
type EventUpdater interface {
	Create(id []byte, owner []byte, normalisedBalance *big.Int, depth, bucketDepth uint8, immutable bool, txHash []byte) error
	TopUp(id []byte, normalisedBalance *big.Int, txHash []byte) error
	UpdateDepth(id []byte, depth uint8, normalisedBalance *big.Int, txHash []byte) error
	UpdatePrice(price *big.Int, txHash []byte) error
	UpdateBlockNumber(blockNumber uint64) error
	Start(startBlock uint64) (<-chan struct{}, error)

	TransactionStart() error
	TransactionEnd() error
}

// UnreserveIteratorFn is used as a callback on Storer.Unreserve method calls.
type UnreserveIteratorFn func(id []byte, radius uint8) (bool, error)

// Storer represents the persistence layer for batches
// on the current (highest available) block.
type Storer interface {
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

	// GetReserveState returns a copy of stored reserve state.
	GetReserveState() *ReserveState

	// SetRadiusSetter sets the RadiusSetter to the given value.
	// The given RadiusSetter will be called when radius changes.
	SetRadiusSetter(RadiusSetter)

	// Unreserve evict batches from the unreserve queue of the storage.
	// During the eviction process, the given UnreserveIteratorFn is called.
	Unreserve(UnreserveIteratorFn) error

	// Reset resets chain state and reserve state of the storage.
	Reset() error
}

// RadiusSetter is used as a callback when the radius of a node changes.
type RadiusSetter interface {
	SetRadius(uint8)
}

// Listener provides a blockchain event iterator.
type Listener interface {
	io.Closer
	Listen(from uint64, updater EventUpdater) <-chan struct{}
}

type BatchEventListener interface {
	HandleCreate(*Batch) error
	HandleTopUp(id []byte, newBalance *big.Int)
	HandleDepthIncrease(id []byte, newDepth uint8, normalisedBalance *big.Int)
}
