// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux || !amd64 || purego

package bmt

// NewPool returns a BMT pool. On this platform only the goroutine implementation
// is compiled in, so SIMDOptIn() is ignored.
func NewPool(c *Conf) Pool {
	return newGoroutinePool(c)
}

// NewHasher returns a standalone (non-pooled) BMT hasher.
func NewHasher() Hasher {
	return newGoroutineHasher()
}

// NewPrefixHasher returns a standalone BMT hasher with the given prefix.
func NewPrefixHasher(prefix []byte) Hasher {
	return newGoroutinePrefixHasher(prefix)
}
