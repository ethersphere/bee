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

// Hasher provides the necessary extension of the hash interface to add the length-prefix of the BMT hash.
//
// Any implementation should make it possible to generate a BMT hash using the hash.Hash interface only.
// However, the limitation will be that the Span of the BMT hash always must be limited to the amount of bytes actually written.
//
// Inclusion-proof generation (Proof / Verify / zero-padded Sum) is NOT part of
// this interface — it lives on the Prover type, which is always goroutine-backed
// regardless of SIMDOptIn. See pkg/bmt/proof.go.
type Hasher interface {
	hash.Hash

	// SetHeaderInt64 sets the header bytes of BMT hash to the little endian binary representation of the int64 argument.
	SetHeaderInt64(int64)

	// SetHeader sets the header bytes of BMT hash by copying the first 8 bytes of the argument.
	SetHeader([]byte)

	// Capacity returns the maximum amount of bytes that will be processed by the implementation.
	Capacity() int
}
