// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
	"github.com/syndtr/goleveldb/leveldb"
)

func TestPinning(t *testing.T) {
	chunks := generateTestRandomChunks(21)
	// SOrt the addresses
	var addresses []string
	for _, c := range chunks {
		addresses = append(addresses, c.Address().String())
	}
	sort.Strings(addresses)

	t.Run("empty-db", func(t *testing.T) {
		db := newTestDB(t, nil)
		// Nothing should be there in the pinned DB
		_, err := db.PinnedChunks(context.Background(), swarm.NewAddress([]byte{0}))
		if err != nil {
			if !errors.Is(err, leveldb.ErrNotFound) {
				t.Fatal(err)
			}
		}
	})

	t.Run("get-pinned-chunks", func(t *testing.T) {
		db := newTestDB(t, nil)

		err := db.Set(context.Background(), storage.ModeSetPin, chunkAddresses(chunks)...)
		if err != nil {
			t.Fatal(err)
		}

		pinnedChunks, err := db.PinnedChunks(context.Background(), swarm.NewAddress([]byte{0}))
		if err != nil {
			t.Fatal(err)
		}

		if pinnedChunks == nil || len(pinnedChunks) != maxChunksToDisplay {
			t.Fatal(err)
		}

		// Check if they are sorted
		for i, addr := range pinnedChunks {
			if addresses[i] != addr.Address.String() {
				t.Fatal("error in getting sorted address")
			}
		}
	})
}

func TestPinInfo(t *testing.T) {
	chunk := generateTestRandomChunk()
	t.Run("get-pinned-chunks", func(t *testing.T) {
		db := newTestDB(t, nil)

		// pin once
		err := db.Set(context.Background(), storage.ModeSetPin, swarm.NewAddress(chunk.Address().Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		pinCounter, err := db.PinInfo(swarm.NewAddress(chunk.Address().Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		if pinCounter != 1 {
			t.Fatal(err)
		}

		// pin twice
		err = db.Set(context.Background(), storage.ModeSetPin, swarm.NewAddress(chunk.Address().Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		pinCounter, err = db.PinInfo(swarm.NewAddress(chunk.Address().Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		if pinCounter != 2 {
			t.Fatal(err)
		}
	})

	t.Run("get-unpinned-chunks", func(t *testing.T) {
		db := newTestDB(t, nil)

		// pin once
		err := db.Set(context.Background(), storage.ModeSetPin, swarm.NewAddress(chunk.Address().Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		pinCounter, err := db.PinInfo(swarm.NewAddress(chunk.Address().Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		if pinCounter != 1 {
			t.Fatal(err)
		}

		// unpin and see if it doesn't exists
		err = db.Set(context.Background(), storage.ModeSetUnpin, swarm.NewAddress(chunk.Address().Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.PinInfo(swarm.NewAddress(chunk.Address().Bytes()))
		if err != nil {
			if !errors.Is(err, leveldb.ErrNotFound) {
				t.Fatal(err)
			}
		}
	})

}
