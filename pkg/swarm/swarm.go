// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package swarm contains most basic and general Swarm concepts.
package swarm

import "encoding/hex"

import "bytes"

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

// Equal returns true if two addresses are identical.
func (a Address) Equal(b Address) bool {
	return bytes.Equal(a, b)
}

// IsZero returns true if the Address is not set to any value.
func (a Address) IsZero() bool {
	return a.Equal(ZeroAddress)
}

// ZeroAddress is the address that has no value.
var ZeroAddress = Address(nil)
