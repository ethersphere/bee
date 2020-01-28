// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package swarm contains most basic and general Swarm concepts.
package swarm

import "encoding/hex"

// Address represents an address in Swarm metric space of
// Node and Chunk addresses.
type Address []byte

// NewAddress returns an Address from a hex-encoded string representation.
func NewAddress(s string) (Address, error) {
	return hex.DecodeString(s)
}

// String returns a hex-encoded representation of the Address.
func (a Address) String() string {
	return hex.EncodeToString(a)
}
