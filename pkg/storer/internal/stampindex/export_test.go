// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package stampindex

import "github.com/ethersphere/bee/v2/pkg/swarm"

var (
	ErrStampItemMarshalNamespaceInvalid  = errStampItemMarshalScopeInvalid
	ErrStampItemMarshalBatchIndexInvalid = errStampItemMarshalBatchIndexInvalid
	ErrStampItemMarshalBatchIDInvalid    = errStampItemMarshalBatchIDInvalid
	ErrStampItemUnmarshalInvalidSize     = errStampItemUnmarshalInvalidSize
)

// NewItemWithValues creates a new Item with given values and fixed keys.
func NewItemWithValues(batchTimestamp []byte, chunkAddress swarm.Address) *Item {
	return &Item{
		scope:      []byte("test_namespace"),
		BatchID:    []byte{swarm.HashSize - 1: 9},
		StampIndex: []byte{swarm.StampIndexSize - 1: 9},
		StampHash:  swarm.EmptyAddress.Bytes(),

		StampTimestamp: batchTimestamp,
		ChunkAddress:   chunkAddress,
	}
}

// NewItemWithKeys creates a new Item with given keys and zero values.
func NewItemWithKeys(namespace string, batchID, batchIndex, stampHash []byte) *Item {
	return &Item{
		scope:      append([]byte(nil), namespace...),
		BatchID:    batchID,
		StampIndex: batchIndex,
		StampHash:  stampHash,
	}
}
