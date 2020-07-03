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
	SectionSize       = 32
	Branches          = 128
	ChunkSize         = SectionSize * Branches
	HashSize          = 32
	MaxPO       uint8 = 15
	MaxBins           = MaxPO + 1
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

type Chunk interface {
	Address() Address
	Data() []byte
	PinCounter() uint64
	WithPinCounter(p uint64) Chunk
	TagID() uint32
	WithTagID(t uint32) Chunk
	Equal(Chunk) bool
}

type chunk struct {
	addr       Address
	sdata      []byte
	pinCounter uint64
	tagID      uint32
}

func NewChunk(addr Address, data []byte) Chunk {
	return &chunk{
		addr:  addr,
		sdata: data,
	}
}

func (c *chunk) WithPinCounter(p uint64) Chunk {
	c.pinCounter = p
	return c
}

func (c *chunk) WithTagID(t uint32) Chunk {
	c.tagID = t
	return c
}

func (c *chunk) Address() Address {
	return c.addr
}

func (c *chunk) Data() []byte {
	return c.sdata
}

func (c *chunk) PinCounter() uint64 {
	return c.pinCounter
}

func (c *chunk) TagID() uint32 {
	return c.tagID
}

func (c *chunk) String() string {
	return fmt.Sprintf("Address: %v Chunksize: %v", c.addr.String(), len(c.sdata))
}

func (c *chunk) Equal(cp Chunk) bool {
	return c.Address().Equal(cp.Address()) && bytes.Equal(c.Data(), cp.Data())
}

type ChunkValidator interface {
	Validate(ch Chunk) (valid bool)
}
