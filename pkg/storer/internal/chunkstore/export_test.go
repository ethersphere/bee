// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore

import storage "github.com/ethersphere/bee/pkg/storage"

type (
	RetrievalIndexItem = retrievalIndexItem
)

var (
	ErrMarshalInvalidRetrievalIndexItemAddress = errMarshalInvalidRetrievalIndexAddress
	ErrUnmarshalInvalidRetrievalIndexItemSize  = errUnmarshalInvalidRetrievalIndexSize
)

func New(store storage.Store, sharky Sharky) storage.ChunkStore {
	return &chunkStoreWrapper{store: store, sharky: sharky}
}
