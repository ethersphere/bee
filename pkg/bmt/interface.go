// Copyright 2021 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bmt

import (
	"hash"
)

const (
	SpanSize = 8
)

// Hash provides the necessary extension of the hash interface to add the length-prefix of the BMT hash.
//
// Any implementation should make it possible to generate a BMT hash using the hash.Hash interface only.
// However, the limitation will be that the Span of the BMT hash always must be limited to the amount of bytes actually written.
type Hash interface {
	hash.Hash

	// SetHeaderInt64 sets the header bytes of BMT hash to the little endian binary representation of the int64 argument.
	SetHeaderInt64(int64)

	// SetHeader sets the header bytes of BMT hash by copying the first 8 bytes of the argument.
	SetHeader([]byte)

	// Hash calculates the BMT hash of the buffer written so far and appends it to the argument
	Hash([]byte) ([]byte, error)

	// Capacity returns the maximum amount of bytes that will be processed by the implementation.
	Capacity() int
}
