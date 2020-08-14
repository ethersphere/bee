// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/syndtr/goleveldb/leveldb"
)

const (
	maxChunksToDisplay = 20 // no of items to display per request
)

// PinnedChunks for now returns the first few pinned chunks to display along with their pin counter.
// TODO: have pagination and prefix filter
func (db *DB) PinnedChunks(ctx context.Context, cursor swarm.Address) (pinnedChunks []*storage.Pinner, err error) {
	count := 0

	var prefix []byte
	if bytes.Equal(cursor.Bytes(), []byte{0}) {
		prefix = nil
	}

	c, err := db.pinIndex.Count()
	if err != nil {
		return nil, fmt.Errorf("list pinned chunks: %w", err)
	}

	// send empty response if there is nothing pinned
	if c == 0 {
		return pinnedChunks, nil
	}

	it, err := db.pinIndex.First(prefix)
	if err != nil {
		return nil, fmt.Errorf("get first pin: %w", err)
	}
	err = db.pinIndex.Iterate(func(item shed.Item) (stop bool, err error) {
		pinnedChunks = append(pinnedChunks,
			&storage.Pinner{
				Address:    swarm.NewAddress(item.Address),
				PinCounter: item.PinCounter,
			})
		count++
		if count >= maxChunksToDisplay {
			return true, nil
		} else {
			return false, nil
		}

	}, &shed.IterateOptions{
		StartFrom:         &it,
		SkipStartFromItem: false,
	})
	return pinnedChunks, err
}

// PinInfo returns the pin counter for a given swarm address, provided that the
// address has been pinned.
func (db *DB) PinInfo(address swarm.Address) (uint64, error) {
	out, err := db.pinIndex.Get(shed.Item{
		Address: address.Bytes(),
	})

	if err != nil {
		if errors.Is(err, leveldb.ErrNotFound) {
			return 0, storage.ErrNotFound
		}
		return 0, err
	}
	return out.PinCounter, nil
}
