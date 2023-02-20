// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testutil

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func RandBytes(t *testing.T, size int) []byte {
	t.Helper()

	buf := make([]byte, size)
	n, err := rand.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, size, n)

	return buf
}
