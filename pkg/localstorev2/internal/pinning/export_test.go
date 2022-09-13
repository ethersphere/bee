// Copyright 2022 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pinstore

import (
	"fmt"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
)

type PinCollectionItem = pinCollectionItem

var (
	ErrInvalidPinCollectionItemAddr = errInvalidPinCollectionAddr
	ErrInvalidPinCollectionItemUUID = errInvalidPinCollectionUUID
	ErrInvalidPinCollectionItemSize = errInvalidPinCollectionSize
	ErrPutterAlreadyClosed          = errPutterAlreadyClosed
)

var NewUUID = newUUID

func IterateCollection(st storage.Store, root swarm.Address, fn func(addr swarm.Address) (bool, error)) error {
	collection := &pinCollectionItem{Addr: root}
	err := st.Get(collection)
	if err != nil {
		return fmt.Errorf("pin store: failed getting collection: %w", err)
	}

	return st.Iterate(storage.Query{
		Factory:       func() storage.Item { return &pinChunkItem{UUID: collection.UUID} },
		ItemAttribute: storage.QueryItemID,
	}, func(r storage.Result) (bool, error) {
		addr := swarm.NewAddress([]byte(r.ID))
		stop, err := fn(addr)
		if err != nil {
			return true, err
		}
		return stop, nil
	})
}

func GetStat(st storage.Store, root swarm.Address) (CollectionStat, error) {
	collection := &pinCollectionItem{Addr: root}
	err := st.Get(collection)
	if err != nil {
		return CollectionStat{}, fmt.Errorf("pin store: failed getting collection: %w", err)
	}

	return collection.Stat, nil
}
