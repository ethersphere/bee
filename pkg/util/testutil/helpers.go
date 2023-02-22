// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testutil

import (
	"crypto/rand"
	"io"
	"reflect"
	"testing"
)

// RandBytes returns bytes slice of specified size filled with random values.
func RandBytes(t *testing.T, size int) []byte {
	t.Helper()

	buf := make([]byte, size)
	n, err := rand.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != size {
		t.Fatalf("expected to read %d, got %d", size, n)
	}

	return buf
}

// CleanupCloser adds Cleanup function to Test which will close supplied Closers.
func CleanupCloser(t *testing.T, closers ...io.Closer) {
	t.Helper()

	t.Cleanup(func() {
		for _, c := range closers {
			if c == nil {
				continue
			}

			if err := c.Close(); err != nil {
				t.Fatalf("failed to gracefully close %s: %s", reflect.TypeOf(c), err)
			}
		}
	})
}
