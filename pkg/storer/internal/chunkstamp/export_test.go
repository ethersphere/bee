// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstamp

import "github.com/ethersphere/bee/v2/pkg/swarm"

func (i *Item) WithNamespace(ns string) *Item {
	i.scope = []byte(ns)
	return i
}

func (i *Item) WithAddress(addr swarm.Address) *Item {
	i.address = addr
	return i
}

func (i *Item) WithStamp(stamp swarm.Stamp) *Item {
	i.stamp = stamp
	return i
}

var (
	ErrMarshalInvalidChunkStampItemNamespace = errMarshalInvalidChunkStampItemScope
	ErrMarshalInvalidChunkStampItemAddress   = errMarshalInvalidChunkStampItemAddress
	ErrUnmarshalInvalidChunkStampItemAddress = errUnmarshalInvalidChunkStampItemAddress
	ErrMarshalInvalidChunkStampItemStamp     = errMarshalInvalidChunkStampItemStamp
	ErrUnmarshalInvalidChunkStampItemSize    = errUnmarshalInvalidChunkStampItemSize
)
