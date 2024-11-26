// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration_test

import (
	"context"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/log"
	"github.com/ethersphere/bee/v2/pkg/sharky"
	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/storage/leveldbstore"
	chunktest "github.com/ethersphere/bee/v2/pkg/storage/testing"

	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/transaction"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/upload"
	localmigration "github.com/ethersphere/bee/v2/pkg/storer/migration"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/stretchr/testify/assert"
)

func Test_Step_05(t *testing.T) {
	t.Parallel()

	sharkyDir := t.TempDir()
	sharkyStore, err := sharky.New(&dirFS{basedir: sharkyDir}, 1, swarm.SocMaxChunkSize)
	assert.NoError(t, err)

	lstore, err := leveldbstore.New("", nil)
	assert.NoError(t, err)

	store := transaction.NewStorage(sharkyStore, lstore)
	t.Cleanup(func() {
		err := store.Close()
		if err != nil {
			t.Fatalf("Close(): unexpected closing storer: %v", err)
		}
	})

	ctx := context.Background()

	wantCount := func(t *testing.T, st storage.Reader, want int) {
		t.Helper()
		count := 0
		err := upload.IterateAll(st, func(_ storage.Item) (bool, error) {
			count++
			return false, nil
		})
		if err != nil {
			t.Fatalf("iterate upload items: %v", err)
		}
		if count != want {
			t.Fatalf("expected %d upload items, got %d", want, count)
		}
	}

	var tag upload.TagItem
	err = store.Run(context.Background(), func(s transaction.Store) error {
		tag, err = upload.NextTag(s.IndexStore())
		return err
	})
	if err != nil {
		t.Fatalf("create tag: %v", err)
	}

	var putter internal.PutterCloserWithReference
	err = store.Run(context.Background(), func(s transaction.Store) error {
		putter, err = upload.NewPutter(s.IndexStore(), tag.TagID)
		return err
	})
	if err != nil {
		t.Fatalf("create putter: %v", err)
	}

	chunks := chunktest.GenerateTestRandomChunks(10)

	err = store.Run(context.Background(), func(s transaction.Store) error {
		for _, ch := range chunks {
			err := putter.Put(ctx, s, ch)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("put chunk: %v", err)
	}

	err = store.Run(ctx, func(s transaction.Store) error {
		return putter.Close(s.IndexStore(), swarm.RandAddress(t))
	})
	if err != nil {
		t.Fatalf("close putter: %v", err)
	}

	wantCount(t, store.IndexStore(), 10)

	err = localmigration.Step_05(store, log.Noop)()
	if err != nil {
		t.Fatalf("step 05: %v", err)
	}
	wantCount(t, store.IndexStore(), 0)
}
