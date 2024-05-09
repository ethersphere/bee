// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pinstore

import (
	"fmt"

	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type (
	PinCollectionItem = pinCollectionItem
	PinChunkItem      = pinChunkItem
	DirtyCollection   = dirtyCollection
)

var (
	ErrInvalidPinCollectionItemAddr = errInvalidPinCollectionAddr
	ErrInvalidPinCollectionItemUUID = errInvalidPinCollectionUUID
	ErrInvalidPinCollectionItemSize = errInvalidPinCollectionSize
	ErrPutterAlreadyClosed          = errPutterAlreadyClosed
	ErrCollectionRootAddressIsZero  = errCollectionRootAddressIsZero
)

var NewUUID = newUUID

func GetStat(st storage.Reader, root swarm.Address) (CollectionStat, error) {
	collection := &pinCollectionItem{Addr: root}
	err := st.Get(collection)
	if err != nil {
		return CollectionStat{}, fmt.Errorf("pin store: failed getting collection: %w", err)
	}

	return collection.Stat, nil
}
