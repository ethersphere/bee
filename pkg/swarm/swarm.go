// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package swarm contains most basic and general Swarm concepts.
package swarm

import (
	"bytes"
	"encoding"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"golang.org/x/crypto/sha3"
)

const (
	SpanSize                = 8
	SectionSize             = 32
	Branches                = 128
	EncryptedBranches       = Branches / 2
	BmtBranches             = 128
	ChunkSize               = SectionSize * Branches
	HashSize                = 32
	MaxPO             uint8 = 31
	ExtendedPO        uint8 = MaxPO + 5
	MaxBins                 = MaxPO + 1
	ChunkWithSpanSize       = ChunkSize + SpanSize
)

var (
	NewHasher = sha3.NewLegacyKeccak256
)

var (
	ErrInvalidChunk = errors.New("invalid chunk")
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

// MemberOf returns true if the address is a member of the
// provided set.
func (a Address) MemberOf(addrs []Address) bool {
	for _, v := range addrs {
		if v.Equal(a) {
			return true
		}
	}
	return false
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

// AddressIterFunc is a callback on every address that is found by the iterator.
type AddressIterFunc func(address Address) error

type Chunk interface {
	// Address returns the chunk address.
	Address() Address
	// Data returns the chunk data.
	Data() []byte
	// TagID returns the tag ID for this chunk.
	TagID() uint32
	// WithTagID attaches the tag ID to the chunk.
	WithTagID(t uint32) Chunk
	// Stamp returns the postage stamp associated with this chunk.
	Stamp() Stamp
	// WithStamp attaches a postage stamp to the chunk.
	WithStamp(Stamp) Chunk
	// Radius is the PO above which the batch is preserved.
	Radius() uint8
	// Depth returns the batch depth of the stamp - allowed batch size = 2^{depth}.
	Depth() uint8
	// WithBatch attaches batch parameters to the chunk.
	WithBatch(radius, depth uint8) Chunk
	// Equal checks if the chunk is equal to another.
	Equal(Chunk) bool
}

// Stamp interface for postage.Stamp to avoid circular dependency
type Stamp interface {
	BatchID() []byte
	Index() []byte
	Sig() []byte
	Timestamp() []byte
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

type chunk struct {
	addr   Address
	sdata  []byte
	tagID  uint32
	stamp  Stamp
	radius uint8
	depth  uint8
}

func NewChunk(addr Address, data []byte) Chunk {
	return &chunk{
		addr:  addr,
		sdata: data,
	}
}

func (c *chunk) WithTagID(t uint32) Chunk {
	c.tagID = t
	return c
}

func (c *chunk) WithStamp(stamp Stamp) Chunk {
	c.stamp = stamp
	return c
}

func (c *chunk) WithBatch(radius, depth uint8) Chunk {
	c.radius = radius
	c.depth = depth
	return c
}

func (c *chunk) Address() Address {
	return c.addr
}

func (c *chunk) Data() []byte {
	return c.sdata
}

func (c *chunk) TagID() uint32 {
	return c.tagID
}

func (c *chunk) Stamp() Stamp {
	return c.stamp
}

func (c *chunk) Radius() uint8 {
	return c.radius
}

func (c *chunk) Depth() uint8 {
	return c.depth
}

func (c *chunk) String() string {
	return fmt.Sprintf("Address: %v Chunksize: %v", c.addr.String(), len(c.sdata))
}

func (c *chunk) Equal(cp Chunk) bool {
	return c.Address().Equal(cp.Address()) && bytes.Equal(c.Data(), cp.Data())
}
