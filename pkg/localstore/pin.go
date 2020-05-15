// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"context"
	"fmt"

	"github.com/ethersphere/bee/pkg/shed"
	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

const (
	maxChunksToDisplay = 20 // no of items to display per request
)

// GetPinnedChunks for now returns the first few pinned chunks to display along with their pin counter.
// TODO: have pagination and prefix filter
func (db *DB) GetPinnedChunks(ctx context.Context, cursor swarm.Address) (pinnedChunks []*storage.PinInfo, err error) {
	count := 0

	it, err := db.pinIndex.First(cursor.Bytes())
	if err != nil {
		return nil, fmt.Errorf("pin chunks: %w", err)
	}
	err = db.pinIndex.Iterate(func(item shed.Item) (stop bool, err error) {
		pi := &storage.PinInfo{
			Address:    swarm.NewAddress(item.Address),
			PinCounter: item.PinCounter,
		}
		pinnedChunks = append(pinnedChunks, pi)
		count++
		if count > maxChunksToDisplay {
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

// GetPinInfo returns the pin counter given a swarm address, provided that the
// address has to be pinned already.
func (db *DB) GetPinInfo(address swarm.Address) (uint64, error) {
	it := shed.Item{
		Address: address.Bytes(),
	}
	out, err := db.pinIndex.Get(it)
	if err != nil {
		return 0, err
	}
	return out.PinCounter, nil
}
