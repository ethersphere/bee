// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux && amd64 && !purego

package keccak

// Stack reserved by the assembly stubs for the XKCP C code. The stubs move SP
// to the top of their frame before calling into C, so the C code (which has no
// stack checks of its own) runs entirely inside this reservation. The sizes are
// exported to the assembly through go_asm.h (the TEXT frame sizes must be
// literals for go vet and are checked to match) and are verified against the
// measured stack usage of the linked .syso blobs by TestStackUsage. With the
// current blobs the worst case, over all SP alignments, is 2000 bytes for x4
// and 2736 bytes for x8, independent of input length.
const (
	keccak256x4FrameSize = 4096
	keccak256x8FrameSize = 4096
)

//go:noescape
func keccak256x4(inputs *[4][]byte, outputs *[4]Hash256)

//go:noescape
func keccak256x8(inputs *[8][]byte, outputs *[8]Hash256)
