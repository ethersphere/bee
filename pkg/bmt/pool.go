// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt

// Pool is the interface all BMT hasher pools satisfy.
type Pool interface {
	// Get returns a BMT hasher, possibly reusing one from the pool.
	Get() Hasher
	// Put returns a hasher to the pool for reuse.
	Put(Hasher)
}

// Conf captures the user-facing configuration of a BMT hasher pool.
// Implementation-specific derived state (tree depth, batch width, etc.)
// is computed internally by each pool implementation.
type Conf struct {
	SegmentCount int
	Capacity     int
	Prefix       []byte
}

// NewConf returns a new Conf for the given segment count and pool capacity.
func NewConf(segmentCount, capacity int) *Conf {
	return &Conf{SegmentCount: segmentCount, Capacity: capacity}
}

// NewConfWithPrefix is like NewConf but with an optional prefix prepended to every hash operation.
func NewConfWithPrefix(prefix []byte, segmentCount, capacity int) *Conf {
	return &Conf{SegmentCount: segmentCount, Capacity: capacity, Prefix: prefix}
}
