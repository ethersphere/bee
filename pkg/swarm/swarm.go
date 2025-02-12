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
)

const (
	StampIndexSize           = 8 // TODO: use this size in related code.
	StampTimestampSize       = 8 // TODO: use this size in related code.
	SpanSize                 = 8
	SectionSize              = 32
	Branches                 = 128
	EncryptedBranches        = Branches / 2
	BmtBranches              = 128
	ChunkSize                = SectionSize * Branches
	HashSize                 = 32
	MaxPO              uint8 = 31
	ExtendedPO         uint8 = MaxPO + 5
	MaxBins                  = MaxPO + 1
	ChunkWithSpanSize        = ChunkSize + SpanSize
	SocSignatureSize         = 65
	SocMinChunkSize          = HashSize + SocSignatureSize + SpanSize
	SocMaxChunkSize          = SocMinChunkSize + ChunkSize
)

var (
	ErrInvalidChunk = errors.New("invalid chunk")
)

var (
	// Ethereum Address for SOC owner of Dispersed Replicas
	// generated from private key 0x0100000000000000000000000000000000000000000000000000000000000000
	ReplicasOwner, _ = hex.DecodeString("dc5b20847f43d67928f49cd4f85d696b5a7617b5")
)

var (
	// EmptyAddress is the address that is all zeroes.
	EmptyAddress = NewAddress(make([]byte, HashSize))
	// ZeroAddress is the address that has no value.
	ZeroAddress = NewAddress(nil)
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
	return ContainsAddress(addrs, a)
}

// IsZero returns true if the Address is not set to any value.
func (a Address) IsZero() bool {
	return a.Equal(ZeroAddress)
}

// IsEmpty returns true if the Address is all zeroes.
func (a Address) IsEmpty() bool {
	return a.Equal(EmptyAddress)
}

// IsValidLength returns true if the Address is of valid length.
func (a Address) IsValidLength() bool {
	return len(a.b) == HashSize
}

// IsValidNonEmpty returns true if the Address has valid length and is not empty.
func (a Address) IsValidNonEmpty() bool {
	return a.IsValidLength() && !a.IsEmpty()
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

// Closer returns if x is closer to a than y
func (x Address) Closer(a Address, y Address) (bool, error) {
	cmp, err := DistanceCmp(a, x, y)
	return cmp == 1, err
}

// Clone returns a new swarm address which is a copy of this one.
func (a Address) Clone() Address {
	if a.b == nil {
		return Address{}
	}
	return Address{b: append(make([]byte, 0, len(a.b)), a.Bytes()...)}
}

// Compare returns an integer comparing two addresses lexicographically.
// The result will be 0 if a == b, -1 if a < b, and +1 if a > b.
func (a Address) Compare(b Address) int {
	return bytes.Compare(a.b, b.b)
}

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
	// Depth returns the batch depth of the stamp - allowed batch size = 2^{depth}.
	Depth() uint8
	// BucketDepth returns the bucket depth of the batch of the stamp - always < depth.
	BucketDepth() uint8
	// Immutable returns whether the batch is immutable
	Immutable() bool
	// WithBatch attaches batch parameters to the chunk.
	WithBatch(depth, bucketDepth uint8, immutable bool) Chunk
	// Equal checks if the chunk is equal to another.
	Equal(Chunk) bool
}

// ChunkType indicates different categories of chunks.
type ChunkType uint8

// String implements Stringer interface.
func (ct ChunkType) String() string {
	switch ct {
	case ChunkTypeContentAddressed:
		return "CAC"
	case ChunkTypeSingleOwner:
		return "SOC"
	default:
		return "unspecified"
	}
}

// DO NOT CHANGE ORDER
const (
	ChunkTypeUnspecified ChunkType = iota
	ChunkTypeContentAddressed
	ChunkTypeSingleOwner
)

// Stamp interface for postage.Stamp to avoid circular dependency
type Stamp interface {
	BatchID() []byte
	Index() []byte
	Sig() []byte
	Timestamp() []byte
	Clone() Stamp
	Hash() ([]byte, error)
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

type chunk struct {
	addr        Address
	sdata       []byte
	tagID       uint32
	stamp       Stamp
	depth       uint8
	bucketDepth uint8
	immutable   bool
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

func (c *chunk) WithBatch(depth, bucketDepth uint8, immutable bool) Chunk {
	c.depth = depth
	c.bucketDepth = bucketDepth
	c.immutable = immutable
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

func (c *chunk) Depth() uint8 {
	return c.depth
}

func (c *chunk) BucketDepth() uint8 {
	return c.bucketDepth
}

func (c *chunk) Immutable() bool {
	return c.immutable
}

func (c *chunk) String() string {
	return fmt.Sprintf("Address: %v Chunksize: %v", c.addr.String(), len(c.sdata))
}

func (c *chunk) Equal(cp Chunk) bool {
	return c.Address().Equal(cp.Address()) && bytes.Equal(c.Data(), cp.Data())
}

var errBadCharacter = errors.New("bad character in binary address")

// ParseBitStrAddress parses overlay addresses in binary format (eg: 111101101) to it's corresponding overlay address.
func ParseBitStrAddress(src string) (Address, error) {

	bitPos := 7
	b := uint8(0)

	var a []byte

	for _, s := range src {
		if s == '1' {
			b |= 1 << bitPos
		} else if s != '0' {
			return ZeroAddress, errBadCharacter
		}
		bitPos--
		if bitPos < 0 {
			a = append(a, b)
			b = 0
			bitPos = 7
		}
	}

	a = append(a, b)

	return bytesToAddr(a), nil
}

func bytesToAddr(b []byte) Address {
	addr := make([]byte, HashSize)
	copy(addr, b)
	return NewAddress(addr)
}

type Neighborhood struct {
	b []byte
	r uint8
}

func NewNeighborhood(a Address, bits uint8) Neighborhood {
	return Neighborhood{b: a.b, r: bits}
}

// String returns a bit string of the Neighborhood.
func (n Neighborhood) String() string {
	return bitStr(n.b, n.r)
}

// Equal returns true if two neighborhoods are identical.
func (n Neighborhood) Equal(b Neighborhood) bool {
	return bytes.Equal(n.b, b.b)
}

// Bytes returns bytes representation of the Neighborhood.
func (n Neighborhood) Bytes() []byte {
	return n.b
}

// Bytes returns bytes representation of the Neighborhood.
func (n Neighborhood) Clone() Neighborhood {
	if n.b == nil {
		return Neighborhood{}
	}
	return Neighborhood{b: append(make([]byte, 0, len(n.b)), n.Bytes()...), r: n.r}
}

func bitStr(src []byte, bits uint8) string {

	ret := ""

	for _, b := range src {
		for i := 7; i >= 0; i-- {
			if b&(1<<i) > 0 {
				ret += "1"
			} else {
				ret += "0"
			}
			bits--
			if bits == 0 {
				return ret
			}
		}
	}

	return ret
}
