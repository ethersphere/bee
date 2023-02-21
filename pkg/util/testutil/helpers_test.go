// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testutil_test

import (
	"testing"

	"github.com/ethersphere/bee/pkg/util/testutil"
)

func TestRandBytes(t *testing.T) {
	t.Parallel()

	const size = 32

	bytes := testutil.RandBytes(t, size)

	if got := len(bytes); got != size {
		t.Fatalf("expected %d, got %d", size, got)
	}
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
		if got := len(c1); got != 1 {
			t.Fatalf("expected %d, got %d", 1, got)
		}
		if got := len(c2); got != 1 {
			t.Fatalf("expected %d, got %d", 1, got)
		}
	})

	testutil.CleanupCloser(t,
		nil,           // nil closers should be allowed
		newCloser(c1), // create closer which will write to chan c1
		newCloser(c2), // create closer which will write to chan c2
	)
}

type closerFn func() error

func (c closerFn) Close() error { return c() }
