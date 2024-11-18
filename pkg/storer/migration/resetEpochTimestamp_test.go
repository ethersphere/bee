// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage/inmemstore"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/reserve"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	localmigration "github.com/ethersphere/bee/v2/pkg/storer/migration"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ResetEpochTimestamp(t *testing.T) {
	t.Parallel()

	sharkyDir := t.TempDir()
	sharkyStore, err := sharky.New(&dirFS{basedir: sharkyDir}, 1, swarm.SocMaxChunkSize)
	assert.NoError(t, err)
	store := inmemstore.New()
	storage := transaction.NewStorage(sharkyStore, store)
	testutil.CleanupCloser(t, storage)

	err = storage.Run(context.Background(), func(s transaction.Store) error {
		return s.IndexStore().Put(&reserve.EpochItem{Timestamp: uint64(time.Now().Second())})
	})
	require.NoError(t, err)

	has, err := storage.IndexStore().Has(&reserve.EpochItem{})
	require.NoError(t, err)
	if !has {
		t.Fatal("epoch item should exist")
	}

	err = localmigration.ResetEpochTimestamp(storage)()
	require.NoError(t, err)

	has, err = storage.IndexStore().Has(&reserve.EpochItem{})
	require.NoError(t, err)
	if has {
		t.Fatal("epoch item should be deleted")
	}
}
