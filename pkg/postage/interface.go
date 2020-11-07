// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"math/big"
)

// EventUpdater interface definitions reflect the updates triggered by events
// emitted by the postage contract on the blockchain
type EventUpdater interface {
	Create(id []byte, owner []byte, amount *big.Int, depth uint8) error
	TopUp(id []byte, amount *big.Int) error
	UpdateDepth(id []byte, depth uint8) error
	UpdatePrice(price *big.Int) error
}

// Storer represents the persistence layer for batches on the current
// (highest available) block
type Storer interface {
	Get(id []byte) (*Batch, error)
	Put(*Batch) error
	PutChainState(*ChainState) error
	GetChainState() (*ChainState, error)
}

// Listener provides a blockchain event iterator
type Listener interface {
	// - it starts at block from
	// - it terminates  with no error when quit channel is closed
	// - if the update function returns an error, the call returns with that error
	// TODO: remove from, do not leak blockchain internals
	Listen(from uint64, updater EventUpdater, quit chan struct{}) error
}
