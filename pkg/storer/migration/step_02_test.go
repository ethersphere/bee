// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration_test

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	storage "github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/cache"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	localmigration "github.com/ethersphere/bee/v2/pkg/storer/migration"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

type testEntry struct {
	address swarm.Address
}

func (e *testEntry) Namespace() string { return "cacheEntry" }

func (e *testEntry) ID() string { return e.address.ByteString() }

func (e *testEntry) Marshal() ([]byte, error) {
	buf := make([]byte, 32*3)
	_, _ = rand.Read(buf)
	return buf, nil
}

func (e *testEntry) Unmarshal(buf []byte) error {
	return nil
}

func (e *testEntry) Clone() storage.Item {
	return &testEntry{
		address: e.address,
	}
}

func (e testEntry) String() string {
	return "testEntry"
}

func Test_Step_02(t *testing.T) {
	t.Parallel()

	stepFn := localmigration.Step_02
	store := internal.NewInmemStorage()

	// simulate old cacheEntryItem with some random bytes.
	var addrs []*testEntry
	for i := 0; i < 10; i++ {
		entry := &testEntry{address: swarm.RandAddress(t)}
		addrs = append(addrs, entry)
		err := store.Run(context.Background(), func(s transaction.Store) error {
			return s.IndexStore().Put(entry)
		})
		assert.NoError(t, err)
	}

	assert.NoError(t, stepFn(store)())

	// check if all entries are migrated.
	for _, entry := range addrs {
		cEntry := &cache.CacheEntryItem{Address: entry.address}
		err := store.IndexStore().Get(cEntry)
		assert.NoError(t, err)
		assert.Equal(t, entry.address, cEntry.Address)
		assert.Greater(t, cEntry.AccessTimestamp, int64(0))
	}
}
