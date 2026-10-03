// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux && amd64 && !purego

package keccak

// stackProbe4 and stackProbe8 exist only for TestStackUsage and are removed by
// the linker from binaries that do not reference them. Each fills a 64KB frame
// with a sentinel pattern, calls the real keccak256x4 / keccak256x8 stub with SP
// placed off bytes below the top of that frame, and returns how many bytes
// below the call-site SP were overwritten.

//go:noescape
func stackProbe4(inputs *[4][]byte, outputs *[4]Hash256, off uintptr) uintptr

//go:noescape
func stackProbe8(inputs *[8][]byte, outputs *[8]Hash256, off uintptr) uintptr
