// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testutil_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/util/testutil"
	"github.com/stretchr/testify/assert"
)

func TestRandBytes(t *testing.T) {
	t.Parallel()

	bytes := testutil.RandBytes(t, 32)
	assert.Len(t, bytes, 32)
	assert.NotEmpty(t, bytes)
}
