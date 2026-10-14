// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fingerprint

// Index constants (proposal §4.1, §4.3.5, §6.3).
const (
	// LeafBits is the depth of the buckets below the root range: the root is
	// split into 1<<LeafBits buckets, the smallest ranges with cached summaries.
	LeafBits = 12

	// RootSymbols is the number of coded symbols cached for the root range.
	RootSymbols = 4096
	// Level1Symbols is the number of coded symbols cached for each of the
	// level-1 ranges (one level below the root).
	Level1Symbols = 512
	// BucketSymbols is the number of coded symbols cached for each bucket.
	BucketSymbols = 32

	// ScanLimit is the largest number of elements in a range for which the
	// symbol stream is produced on demand by scanning the address index, which
	// makes the stream of such a range unlimited.
	ScanLimit = 4096
)

// DefaultMapperID is the ID of the default Mapper, the float64 mapper of
// proposal §4.3.4. It is sent in Status.Mapping; peers with different values do
// not run sessions with each other. ID 2 is reserved for the integer mapper.
const DefaultMapperID uint32 = 1
