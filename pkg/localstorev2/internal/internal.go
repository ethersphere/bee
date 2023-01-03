// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal

import (
	"bytes"
	"context"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
)

// Storage groups the storage.Store and storage.ChunkStore interfaces with context..
type Storage interface {
	Ctx() context.Context
	Store() storage.Store
	ChunkStore() storage.ChunkStore
}

// PutterCloserWithReference provides a Putter which can be closed with a root
// swarm reference associated with this session.
type PutterCloserWithReference interface {
	storage.Putter
	Close(swarm.Address) error
}

var emptyAddr = make([]byte, swarm.HashSize)

func AddressOrZero(buf []byte) swarm.Address {
	if bytes.Equal(buf, emptyAddr) {
		return swarm.ZeroAddress
	}
	return swarm.NewAddress(append(make([]byte, 0, swarm.HashSize), buf...))
}

func AddressBytesOrZero(addr swarm.Address) []byte {
	if addr.IsZero() {
		return make([]byte, swarm.HashSize)
	}
	return addr.Bytes()
}
