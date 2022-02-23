// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package localstore

import (
	"context"
	"testing"

	"github.com/ethersphere/bee/pkg/storage"
	"github.com/ethersphere/bee/pkg/swarm"
)

func TestRecovery(t *testing.T) {
	chunkCount := 150

	db := newTestDB(t, &Options{
		Capacity:        100,
		ReserveCapacity: 200,
	})

	loc, _ := recovery(db)

	for range loc {
		t.Fatal("not expecting any locations, found at least one")
	}

	for i := 0; i < chunkCount; i++ {
		ch := generateTestRandomChunkAt(swarm.NewAddress(db.baseKey), 2).WithBatch(5, 3, 2, false)
		_, err := db.Put(context.Background(), storage.ModePutUpload, ch)
		if err != nil {
			t.Fatal(err)
		}
	}

	loc, _ = recovery(db)

	var locationCount int
	for range loc {
		locationCount++
	}

	if locationCount != chunkCount {
		t.Fatalf("want %d chunks, got %d", chunkCount, locationCount)
	}
}
