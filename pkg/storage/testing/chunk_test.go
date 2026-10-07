// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testing_test

import (
	"testing"

	"github.com/ethersphere/bee/v2/pkg/cac"
	"github.com/ethersphere/bee/v2/pkg/soc"
	chunktesting "github.com/ethersphere/bee/v2/pkg/storage/testing"
)

// TestChunkValidityCases checks that every case is classified as its Valid
// field claims by the CAC and SOC validators.
func TestChunkValidityCases(t *testing.T) {
	t.Parallel()

	for _, tc := range chunktesting.ChunkValidityCases(t) {
		if got := cac.Valid(tc.Chunk) || soc.Valid(tc.Chunk); got != tc.Valid {
			t.Errorf("%s: got valid %v, want %v", tc.Name, got, tc.Valid)
		}
	}
}
