// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testutil

import (
	"crypto/rand"
	"io"
	mrand "math/rand"
	"reflect"
	"testing"
)

// RandBytes returns bytes slice of specified size filled with random values.
func RandBytes(tb testing.TB, size int) []byte {
	tb.Helper()

	buf := make([]byte, size)
	n, err := rand.Read(buf)
	if err != nil {
		tb.Fatal(err)
	}
	if n != size {
		tb.Fatalf("expected to read %d, got %d", size, n)
	}

	return buf
}

// RandBytesWithSeed returns bytes slice of specified size filled with random values generated using seed.
func RandBytesWithSeed(tb testing.TB, size int, seed int64) []byte {
	tb.Helper()

	buf := make([]byte, size)

	r := mrand.New(mrand.NewSource(seed))
	n, err := io.ReadFull(r, buf)
	if err != nil {
		tb.Fatal(err)
	}
	if n != size {
		tb.Fatalf("expected to read %d, got %d", size, n)
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
