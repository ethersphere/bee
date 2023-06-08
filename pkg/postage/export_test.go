// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"github.com/ethersphere/bee/pkg/swarm"
)

var (
	IndexToBytes   = indexToBytes
	BlockThreshold = blockThreshold
)

var (
	ErrStampItemMarshalBatchIDInvalid      = errStampItemMarshalBatchIDInvalid
	ErrStampItemMarshalChunkAddressInvalid = errStampItemMarshalChunkAddressInvalid
	ErrStampItemUnmarshalInvalidSize       = errStampItemUnmarshalInvalidSize
)

type StampItem = stampItem

func (si *stampItem) WithBatchID(id []byte) *StampItem {
	si.batchID = id
	return si
}

func (si *stampItem) WithChunkAddress(addr swarm.Address) *StampItem {
	si.chunkAddress = addr
	return si
}

func (si *stampItem) WithBatchIndex(index []byte) *StampItem {
	si.BatchIndex = index
	return si
}

func (si *stampItem) WithBatchTimestamp(timestamp []byte) *StampItem {
	si.BatchTimestamp = timestamp
	return si
}

func NewStampItem() *StampItem {
	return new(stampItem)
}
