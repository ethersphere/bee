// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration_test

import (
	"math/rand"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemstore"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/chunkstore"
	localmigration "github.com/ethersphere/bee/v2/pkg/storer/migration"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/stretchr/testify/assert"
)

func Test_RefCntSize(t *testing.T) {
	t.Parallel()

	stepFn := localmigration.RefCountSizeInc
	store := inmemstore.New()

	// simulate old cacheEntryItem with some random bytes.
	var oldItems []*localmigration.OldRetrievalIndexItem
	for i := 0; i < 10; i++ {
		entry := &localmigration.OldRetrievalIndexItem{
			Address:   swarm.RandAddress(t),
			Timestamp: uint64(rand.Int()),
			Location:  sharky.Location{Shard: uint8(rand.Int()), Slot: uint32(rand.Int()), Length: uint16(rand.Int())},
			RefCnt:    uint8(rand.Int()),
		}
		oldItems = append(oldItems, entry)
		err := store.Put(entry)
		assert.NoError(t, err)
	}

	assert.NoError(t, stepFn(store, log.Noop)())

	// check if all entries are migrated.
	for _, entry := range oldItems {
		cEntry := &chunkstore.RetrievalIndexItem{Address: entry.Address}
		err := store.Get(cEntry)
		assert.NoError(t, err)
		assert.Equal(t, entry.Address, cEntry.Address)
		assert.Equal(t, entry.Timestamp, cEntry.Timestamp)
		assert.Equal(t, entry.Location, cEntry.Location)
		assert.Equal(t, uint32(entry.RefCnt), cEntry.RefCnt)
	}
}
