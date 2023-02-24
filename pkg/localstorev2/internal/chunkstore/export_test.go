// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chunkstore

import storage "github.com/ethersphere/bee/pkg/storagev2"

type (
	RetrievalIndexItem = retrievalIndexItem
)

var (
	ErrMarshalInvalidRetrievalIndexItemAddress = errMarshalInvalidRetrievalIndexAddress
	ErrUnmarshalInvalidRetrievalIndexItemSize  = errUnmarshalInvalidRetrievalIndexSize
)

func (t *txChunkStoreWrapper) Store() storage.Store {
	return t.txStore
}
