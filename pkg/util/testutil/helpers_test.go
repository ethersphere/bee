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

func TestCleanupCloser(t *testing.T) {
	t.Parallel()

	newCloser := func(c chan struct{}) closerFn {
		return func() error {
			c <- struct{}{}
			return nil
		}
	}

	c1 := make(chan struct{}, 1)
	c2 := make(chan struct{}, 1)

	// Test first add it's own Cleanup function which will
	// assert that all Close method is being invoked
	t.Cleanup(func() {
		assert.Len(t, c1, 1)
		assert.Len(t, c2, 1)
	})

	testutil.CleanupCloser(t,
		nil,           // nil closers should be allowed
		newCloser(c1), // create closer which will write to chan c1
		newCloser(c2), // create closer which will write to chan c2
	)
}

type closerFn func() error

func (c closerFn) Close() error { return c() }
