// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var (
	IndexToBytes   = indexToBytes
	BlockThreshold = blockThreshold
	ToBucket       = toBucket
)

var (
	ErrStampItemMarshalBatchIDInvalid      = errStampItemMarshalBatchIDInvalid
	ErrStampItemMarshalChunkAddressInvalid = errStampItemMarshalChunkAddressInvalid
	ErrStampItemUnmarshalInvalidSize       = errStampItemUnmarshalInvalidSize
)

func (si *StampItem) WithBatchID(id []byte) *StampItem {
	si.BatchID = id
	return si
}

func (si *StampItem) WithChunkAddress(addr swarm.Address) *StampItem {
	si.chunkAddress = addr
	return si
}

func (si *StampItem) WithBatchIndex(index []byte) *StampItem {
	si.BatchIndex = index
	return si
}

func (si *StampItem) WithBatchTimestamp(timestamp []byte) *StampItem {
	si.BatchTimestamp = timestamp
	return si
}

func NewStampItem() *StampItem {
	return new(StampItem)
}

func ModifyBuckets(st *StampIssuer, buckets []uint32) {
	st.data.Buckets = buckets
}

func (si *StampIssuer) Increment(addr swarm.Address) ([]byte, []byte, error) {
	return si.increment(addr)
}
