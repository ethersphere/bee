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
	Create(id []byte, owner []byte, normalisedBalance *big.Int, depth uint8) error
	TopUp(id []byte, normalisedBalance *big.Int) error
	UpdateDepth(id []byte, depth uint8, normalisedBalance *big.Int) error
	UpdatePrice(price *big.Int) error
	UpdateBlockNumber(blockNumber uint64) error
	Start(startBlock uint64) (<-chan struct{}, error)

	TransactionStart() error
	TransactionEnd() error
}

// Storer represents the persistence layer for batches on the current (highest
// available) block.
type Storer interface {
	Get(id []byte) (*Batch, error)
	Put(*Batch, *big.Int, uint8) error
	PutChainState(*ChainState) error
	GetChainState() *ChainState
	GetReserveState() *ReserveState
	SetRadiusSetter(RadiusSetter)

	Reset() error
}

type RadiusSetter interface {
	SetRadius(uint8)
}

// Listener provides a blockchain event iterator.
type Listener interface {
	io.Closer
	Listen(from uint64, updater EventUpdater) <-chan struct{}
}
