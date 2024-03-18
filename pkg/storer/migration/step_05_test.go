// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration_test

import (
	"context"
	"testing"

	"github.com/ethersphere/bee/v2/pkg/node"
	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/storage"
	chunktest "github.com/ethersphere/bee/v2/pkg/storage/testing"
	"github.com/ethersphere/bee/v2/pkg/storer"
	"github.com/ethersphere/bee/v2/pkg/storer/internal"
	"github.com/ethersphere/bee/v2/pkg/storer/internal/upload"
	localmigration "github.com/ethersphere/bee/v2/pkg/storer/migration"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	kademlia "github.com/ethersphere/bee/v2/pkg/topology/mock"
	"github.com/ethersphere/bee/v2/pkg/util/testutil"
)

func Test_Step_05(t *testing.T) {
	t.Parallel()

	db, err := storer.New(context.Background(), "", &storer.Options{
		Logger:          testutil.NewLogger(t),
		RadiusSetter:    kademlia.NewTopologyDriver(),
		Batchstore:      new(postage.NoOpBatchStore),
		ReserveCapacity: node.ReserveCapacity,
	})
	if err != nil {
		t.Fatalf("New(...): unexpected error: %v", err)
	}

	t.Cleanup(func() {
		err := db.Close()
		if err != nil {
			t.Fatalf("Close(): unexpected closing storer: %v", err)
		}
	})

	wantCount := func(t *testing.T, st internal.Storage, want int) {
		t.Helper()
		count := 0
		err = upload.IterateAll(st.IndexStore(), func(_ storage.Item) (bool, error) {
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

	err = db.Execute(context.Background(), func(st internal.Storage) error {
		tag, err := upload.NextTag(st.IndexStore())
		if err != nil {
			t.Fatalf("create tag: %v", err)
		}

		putter, err := upload.NewPutter(st, tag.TagID)
		if err != nil {
			t.Fatalf("create putter: %v", err)
		}
		ctx := context.Background()
		chunks := chunktest.GenerateTestRandomChunks(10)
		b, err := st.IndexStore().Batch(ctx)
		if err != nil {
			t.Fatalf("create batch: %v", err)
		}

		for _, ch := range chunks {
			err := putter.Put(ctx, st, b, ch)
			if err != nil {
				t.Fatalf("put chunk: %v", err)
			}
		}
		err = putter.Close(st, st.IndexStore(), swarm.RandAddress(t))
		if err != nil {
			t.Fatalf("close putter: %v", err)
		}
		err = b.Commit()
		if err != nil {
			t.Fatalf("commit batch: %v", err)
		}

		wantCount(t, st, 10)
		err = localmigration.Step_05(st.IndexStore())
		if err != nil {
			t.Fatalf("step 05: %v", err)
		}
		wantCount(t, st, 0)
		return nil
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
}
