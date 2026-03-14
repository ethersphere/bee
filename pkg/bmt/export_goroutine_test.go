// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux || !amd64 || purego

package bmt

// NewConfNoSIMD on the goroutine path just returns a regular Conf
// since there is no SIMD to disable.
func NewConfNoSIMD(segmentCount, capacity int) *Conf {
	return NewConf(segmentCount, capacity)
}
