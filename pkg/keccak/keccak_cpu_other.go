// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !linux || !amd64 || purego

package keccak

// No SIMD Keccak implementations are available on this platform;
// Sum256x4/Sum256x8 will fall back to the scalar goroutine path.
var (
	hasAVX2   = false
	hasAVX512 = false
)
