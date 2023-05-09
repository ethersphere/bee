// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package types

// Trio is type for group of three elements.
type Trio[T any] struct {
	A T
	B T
	C T
}

func NewTrio[T any](a, b, c T) Trio[T] {
	return Trio[T]{
		A: a,
		B: b,
		C: c,
	}
}

func ToByte32(data []byte) [32]byte {
	var res [32]byte
	copy(res[:], data)
	return res
}
