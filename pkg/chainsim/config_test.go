// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chainsim

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultMempoolTTL(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	assert.Equal(t, uint64(0), cfg.MempoolTTL)

	normalized := cfg.normalized()
	assert.Equal(t, uint64(120), normalized.MempoolTTL)
}

func TestDefaultMempoolTTL_12s(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.BlockPeriod = 12 * time.Second
	normalized := cfg.normalized()
	assert.Equal(t, uint64(50), normalized.MempoolTTL)
}

func TestDisabledMempoolTTLNotOverridden(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.MempoolTTL = DisabledMempoolTTL
	normalized := cfg.normalized()
	assert.Equal(t, DisabledMempoolTTL, normalized.MempoolTTL)
}
