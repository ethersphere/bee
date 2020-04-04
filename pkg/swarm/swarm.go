// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package swarm contains most basic and general Swarm concepts.
package swarm

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
)


const (
	DefaultChunkSize     = 4096
	DefaultAddressLength = 20
)

// Address represents an address in Swarm metric space of
// Node and Chunk addresses.
type Address struct {
	b []byte
}

// NewAddress constructs Address from a byte slice.
func NewAddress(b []byte) Address {
	return Address{b: b}
}

// ParseHexAddress returns an Address from a hex-encoded string representation.
func ParseHexAddress(s string) (a Address, err error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return a, err
	}
	return NewAddress(b), nil
}

// MustParseHexAddress returns an Address from a hex-encoded string
// representation, and panics if there is a parse error.
func MustParseHexAddress(s string) Address {
	a, err := ParseHexAddress(s)
	if err != nil {
		panic(err)
	}
	return a
}

// String returns a hex-encoded representation of the Address.
func (a Address) String() string {
	return hex.EncodeToString(a.b)
}

// Equal returns true if two addresses are identical.
func (a Address) Equal(b Address) bool {
	return bytes.Equal(a.b, b.b)
}

// IsZero returns true if the Address is not set to any value.
func (a Address) IsZero() bool {
	return a.Equal(ZeroAddress)
}

// Bytes returns bytes representation of the Address.
func (a Address) Bytes() []byte {
	return a.b
}

// ByteString returns raw Address string without encoding.
func (a Address) ByteString() string {
	return string(a.Bytes())
}

// UnmarshalJSON sets Address to a value from JSON-encoded representation.
func (a *Address) UnmarshalJSON(b []byte) (err error) {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*a, err = ParseHexAddress(s)
	return err
}

// MarshalJSON returns JSON-encoded representation of Address.
func (a Address) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

// ZeroAddress is the address that has no value.
var ZeroAddress = NewAddress(nil)

// Data represents swarm's chunk data which is of 4K in length
type Data struct {
	d []byte
}

// NewData constructs data from a byte slice.
func NewData(b []byte) Data {
	return Data{d: b}
}

// Bytes returns bytes representation of the Data.
func (d Data) Bytes() []byte {
	return d.d
}

// Chunk defines a swarm chunk structure. It contains of the Address and the
// chunk data.
type Chunk struct {
	addr Address
	data Data
}

// NewChunk crates a chunk with the given address and data.
func NewChunk(addr Address, chunkData Data) (chunk Chunk) {
	return Chunk{
		addr: addr,
		data: chunkData,
	}
}

// Address returns the chunk's address.
func (c *Chunk) Address() Address {
	return c.addr
}

// Data returns the chunk's data.
func (c *Chunk) Data() Data {
	return c.data
}

func (c *Chunk) String() string {
	return fmt.Sprintf("Address: %v Chunksize: %v", c.addr.String(), len(c.data.Bytes()))
}