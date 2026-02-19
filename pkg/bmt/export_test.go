// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
package bmt

var Sha3hash = sha3hash

// NewConfNoSIMD creates a Conf identical to NewConf but with SIMD disabled,
// useful for benchmarking the non-SIMD path.
func NewConfNoSIMD(segmentCount, capacity int) *Conf {
	c := NewConf(segmentCount, capacity)
	c.useSIMD = false
	c.batchWidth = 4 // use 4-wide batching with scalar fallback
	return c
}
