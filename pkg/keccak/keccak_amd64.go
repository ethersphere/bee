// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux && amd64 && !purego

package keccak

//go:noescape
func keccak256x4(inputs *[4][]byte, outputs *[4]Hash256)

//go:noescape
func keccak256x8(inputs *[8][]byte, outputs *[8]Hash256)
