// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package storage_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/storage"
)

func TestChunkLocation(t *testing.T) {
	t.Parallel()

	var zero storage.ChunkLocation
	if !zero.IsZero() {
		t.Fatal("expected zero ChunkLocation to return true for IsZero")
	}

	nonZero := storage.ChunkLocation{1, 2, 3}
	if nonZero.IsZero() {
		t.Fatal("expected non-zero ChunkLocation to return false for IsZero")
	}
}
