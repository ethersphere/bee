// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux && amd64 && !purego

package keccak

import cpuid "github.com/klauspost/cpuid/v2"

var (
	hasAVX2   = cpuid.CPU.Supports(cpuid.AVX2)
	hasAVX512 = cpuid.CPU.Supports(cpuid.AVX512F, cpuid.AVX512VL)
)
