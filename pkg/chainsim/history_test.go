// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chainsim

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPruneNonceHistory(t *testing.T) {
	t.Parallel()

	hist := []nonceRecord{
		{blockNum: 1, nonce: 0},
		{blockNum: 5, nonce: 1},
		{blockNum: 10, nonce: 2},
		{blockNum: 15, nonce: 3},
	}
	pruned := pruneNonceHistory(hist, 10)
	require.Equal(t, []nonceRecord{
		{blockNum: 5, nonce: 1},
		{blockNum: 10, nonce: 2},
		{blockNum: 15, nonce: 3},
	}, pruned)
}

func TestPruneNonceHistory_Empty(t *testing.T) {
	t.Parallel()
	require.Nil(t, pruneNonceHistory(nil, 10))
}

func TestPruneNonceHistory_AllBeforeCutoff(t *testing.T) {
	t.Parallel()

	hist := []nonceRecord{{blockNum: 1, nonce: 0}, {blockNum: 2, nonce: 1}}
	pruned := pruneNonceHistory(hist, 100)
	require.Equal(t, []nonceRecord{{blockNum: 2, nonce: 1}}, pruned)
}
