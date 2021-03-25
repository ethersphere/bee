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

	// SetSpan sets the length prefix of BMT hash.
	SetSpan(int64)

	// SetSpanBytes sets the length prefix of BMT hash in byte form.
	SetSpanBytes([]byte)

	// Capacity returns the maximum amount of bytes that will be processed by the implementation.
	Capacity() int

	// WriteSection writes to a specific section of the data to be hashed.
	WriteSection(idx int, data []byte)
}
