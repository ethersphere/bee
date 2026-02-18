// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux && amd64 && !purego

package bmt

// NewConfNoSIMD creates a Conf identical to NewConf but with SIMD disabled,
// useful for benchmarking the non-SIMD path.
func NewConfNoSIMD(segmentCount, capacity int) *Conf {
	c := NewConf(segmentCount, capacity)
	c.useSIMD = false
	c.batchWidth = 8 // use 8-wide batching with scalar fallback
	return c
}
