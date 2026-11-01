// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux && amd64 && !purego

package bmt

// NewSIMDHasher returns a standalone SIMD hasher over segmentCount segments
// that hashes in batches of batchWidth lanes, regardless of the width the CPU
// would pick, so tests can cover the AVX2 path on AVX-512 hardware.
func NewSIMDHasher(prefix []byte, segmentCount, batchWidth int) Hasher {
	sc := newSIMDConf(prefix, segmentCount, 1)
	sc.batchWidth = batchWidth
	return &simdHasher{
		simdConf: sc,
		span:     make([]byte, SpanSize),
		bmt:      newSIMDTree(sc.maxSize, sc.depth, sc.baseHasher, sc.prefix),
	}
}
