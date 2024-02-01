// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package migration_test

import (
	"context"
	"github.com/ethersphere/bee/pkg/swarm"
	"testing"

	"github.com/ethersphere/bee/pkg/storage"
	chunktest "github.com/ethersphere/bee/pkg/storage/testing"
	"github.com/ethersphere/bee/pkg/storer/internal"
	"github.com/ethersphere/bee/pkg/storer/internal/upload"
	localmigration "github.com/ethersphere/bee/pkg/storer/migration"
)

func Test_Step_05(t *testing.T) {
	t.Parallel()

	st, closer := internal.NewInmemStorage()
	t.Cleanup(func() {
		err := closer()
		if err != nil {
			t.Errorf("closing storage: %v", err)
		}
	})

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

	wantCount := func(t *testing.T, want int) {
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

	wantCount(t, 10)
	err = localmigration.Step_05(st.IndexStore())
	if err != nil {
		t.Fatalf("step 05: %v", err)
	}
	wantCount(t, 0)
}
