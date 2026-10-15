// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storer

import (
	"context"

	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// ReserveIterateAddr implements ReserveAddrStore.
func (db *DB) ReserveIterateAddr(prefix swarm.Address, bits uint8, after []byte, cb func(ChunkID) (stop bool, err error)) error {
	return ErrAddrIndexDisabled
}

// ReserveGetByID implements ReserveAddrStore.
func (db *DB) ReserveGetByID(ctx context.Context, addr swarm.Address, stampHash []byte) (swarm.Chunk, error) {
	return nil, ErrAddrIndexDisabled
}

// ReserveHasID implements ReserveAddrStore.
func (db *DB) ReserveHasID(addr swarm.Address, stampHash []byte) (bool, error) {
	return false, ErrAddrIndexDisabled
}

// ReserveAddrIndexReady implements ReserveAddrStore.
func (db *DB) ReserveAddrIndexReady() bool {
	return false
}

// SetReserveObserver implements ReserveAddrStore.
func (db *DB) SetReserveObserver(o ReserveObserver) error {
	return ErrAddrIndexDisabled
}
