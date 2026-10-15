// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve

import (
	"encoding/binary"
	"errors"
	"path"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

var (
	errMarshalInvalidAddress = errors.New("marshal: invalid address")
	errUnmarshalInvalidSize  = errors.New("unmarshal: invalid size")
)

// BatchRadiusItem allows iteration of the chunks with respect to bin and batchID.
// Used for batch evictions of certain bins.
type BatchRadiusItem struct {
	Bin       uint8
	BatchID   []byte
	StampHash []byte
	Address   swarm.Address
	BinID     uint64
}

func (b *BatchRadiusItem) Namespace() string {
	return "batchRadius"
}

// batchID/bin/ChunkAddr/stampHash
func (b *BatchRadiusItem) ID() string {
	return string(b.BatchID) + string(b.Bin) + b.Address.ByteString() + string(b.StampHash)
}

func (b *BatchRadiusItem) String() string {
	return path.Join(b.Namespace(), b.ID())
}

func (b *BatchRadiusItem) Clone() storage.Item {
	if b == nil {
		return nil
	}
	return &BatchRadiusItem{
		Bin:       b.Bin,
		BatchID:   copyBytes(b.BatchID),
		Address:   b.Address.Clone(),
		BinID:     b.BinID,
		StampHash: copyBytes(b.StampHash),
	}
}

const batchRadiusItemSize = 1 + swarm.HashSize + swarm.HashSize + 8 + swarm.HashSize

func (b *BatchRadiusItem) Marshal() ([]byte, error) {
	if b.Address.IsZero() {
		return nil, errMarshalInvalidAddress
	}

	buf := make([]byte, batchRadiusItemSize)

	i := 0

	buf[i] = b.Bin
	i += 1

	copy(buf[i:i+swarm.HashSize], b.BatchID)
	i += swarm.HashSize

	copy(buf[i:i+swarm.HashSize], b.Address.Bytes())
	i += swarm.HashSize

	binary.BigEndian.PutUint64(buf[i:i+8], b.BinID)
	i += 8

	copy(buf[i:i+swarm.HashSize], b.StampHash)
	return buf, nil
}

func (b *BatchRadiusItem) Unmarshal(buf []byte) error {
	if len(buf) != batchRadiusItemSize {
		return errUnmarshalInvalidSize
	}

	i := 0
	b.Bin = buf[i]
	i += 1

	b.BatchID = copyBytes(buf[i : i+swarm.HashSize])
	i += swarm.HashSize

	b.Address = swarm.NewAddress(buf[i : i+swarm.HashSize]).Clone()
	i += swarm.HashSize

	b.BinID = binary.BigEndian.Uint64(buf[i : i+8])
	i += 8

	b.StampHash = copyBytes(buf[i : i+swarm.HashSize])
	return nil
}

// ChunkBinItem allows for iterating on ranges of bin and binIDs for chunks.
// BinIDs come in handy when syncing the reserve contents with other peers.
type ChunkBinItem struct {
	Bin       uint8
	BinID     uint64
	Address   swarm.Address
	BatchID   []byte
	StampHash []byte
	ChunkType swarm.ChunkType
}

func (c *ChunkBinItem) Namespace() string {
	return "chunkBin"
}

// bin/binID
func (c *ChunkBinItem) ID() string {
	return binIDToString(c.Bin, c.BinID)
}

func binIDToString(bin uint8, binID uint64) string {
	binIDBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(binIDBytes, binID)
	return string(bin) + string(binIDBytes)
}

func (c *ChunkBinItem) String() string {
	return path.Join(c.Namespace(), c.ID())
}

func (c *ChunkBinItem) Clone() storage.Item {
	if c == nil {
		return nil
	}
	return &ChunkBinItem{
		Bin:       c.Bin,
		BinID:     c.BinID,
		Address:   c.Address.Clone(),
		BatchID:   copyBytes(c.BatchID),
		StampHash: copyBytes(c.StampHash),
		ChunkType: c.ChunkType,
	}
}

const chunkBinItemSize = 1 + 8 + swarm.HashSize + swarm.HashSize + 1 + swarm.HashSize

func (c *ChunkBinItem) Marshal() ([]byte, error) {
	if c.Address.IsZero() {
		return nil, errMarshalInvalidAddress
	}

	buf := make([]byte, chunkBinItemSize)
	i := 0

	buf[i] = c.Bin
	i += 1

	binary.BigEndian.PutUint64(buf[i:i+8], c.BinID)
	i += 8

	copy(buf[i:i+swarm.HashSize], c.Address.Bytes())
	i += swarm.HashSize

	copy(buf[i:i+swarm.HashSize], c.BatchID)
	i += swarm.HashSize

	buf[i] = uint8(c.ChunkType)
	i += 1

	copy(buf[i:i+swarm.HashSize], c.StampHash)
	return buf, nil
}

func (c *ChunkBinItem) Unmarshal(buf []byte) error {
	if len(buf) != chunkBinItemSize {
		return errUnmarshalInvalidSize
	}

	i := 0
	c.Bin = buf[i]
	i += 1

	c.BinID = binary.BigEndian.Uint64(buf[i : i+8])
	i += 8

	c.Address = swarm.NewAddress(buf[i : i+swarm.HashSize]).Clone()
	i += swarm.HashSize

	c.BatchID = copyBytes(buf[i : i+swarm.HashSize])
	i += swarm.HashSize

	c.ChunkType = swarm.ChunkType(buf[i])
	i += 1

	c.StampHash = copyBytes(buf[i : i+swarm.HashSize])
	return nil
}

// ChunkAddrItem orders the reserve's (chunk, stamp) pairs by chunk address for
// range reconciliation. It is written only when the reserve address index is
// enabled, in the same transaction as the ChunkBinItem of the same pair.
// Address and StampHash form the key; the value holds BatchID and ChunkType,
// so Unmarshal leaves the key fields untouched and an iteration recovers them
// from the result ID.
type ChunkAddrItem struct {
	Address   swarm.Address
	StampHash []byte
	BatchID   []byte
	ChunkType swarm.ChunkType
}

func (c *ChunkAddrItem) Namespace() string {
	return "chunkAddr"
}

// address/stampHash
func (c *ChunkAddrItem) ID() string {
	return c.Address.ByteString() + string(c.StampHash)
}

func (c *ChunkAddrItem) String() string {
	return path.Join(c.Namespace(), c.ID())
}

func (c *ChunkAddrItem) Clone() storage.Item {
	if c == nil {
		return nil
	}
	return &ChunkAddrItem{
		Address:   c.Address.Clone(),
		StampHash: copyBytes(c.StampHash),
		BatchID:   copyBytes(c.BatchID),
		ChunkType: c.ChunkType,
	}
}

const chunkAddrItemSize = swarm.HashSize + 1

// Marshal encodes BatchID (32 bytes) ‖ ChunkType (1 byte).
func (c *ChunkAddrItem) Marshal() ([]byte, error) {
	if c.Address.IsZero() {
		return nil, errMarshalInvalidAddress
	}
	buf := make([]byte, chunkAddrItemSize)
	copy(buf[:swarm.HashSize], c.BatchID)
	buf[swarm.HashSize] = uint8(c.ChunkType)
	return buf, nil
}

// Unmarshal decodes the value into BatchID and ChunkType.
func (c *ChunkAddrItem) Unmarshal(buf []byte) error {
	if len(buf) != chunkAddrItemSize {
		return errUnmarshalInvalidSize
	}
	c.BatchID = copyBytes(buf[:swarm.HashSize])
	c.ChunkType = swarm.ChunkType(buf[swarm.HashSize])
	return nil
}

// ChunkAddrIndexVersion is the version of the address index layout recorded in
// its validity marker.
const ChunkAddrIndexVersion uint8 = 1

// ChunkAddrValidItem marks the ChunkAddrItem namespace as complete. It is
// written last by the address index fill and removed when the index is
// disabled.
//
// Its namespace must not start with the ChunkAddrItem namespace "chunkAddr":
// leveldbstore iterations with PrefixAtStart cover every key with the bare
// namespace as a byte prefix (no separator), so a "chunkAddr…" marker key
// would be reached, and fail to unmarshal, by address index iterations that
// run to the end of the address space.
type ChunkAddrValidItem struct {
	Version uint8
}

func (c *ChunkAddrValidItem) Namespace() string { return "addrIndexValid" }
func (c *ChunkAddrValidItem) ID() string        { return "" }
func (c *ChunkAddrValidItem) String() string    { return c.Namespace() }

func (c *ChunkAddrValidItem) Clone() storage.Item {
	if c == nil {
		return nil
	}
	return &ChunkAddrValidItem{Version: c.Version}
}

func (c *ChunkAddrValidItem) Marshal() ([]byte, error) {
	return []byte{c.Version}, nil
}

func (c *ChunkAddrValidItem) Unmarshal(buf []byte) error {
	if len(buf) != 1 {
		return errUnmarshalInvalidSize
	}
	c.Version = buf[0]
	return nil
}

// BinItem stores the latest binIDs for each bin between 0 and swarm.MaxBins
type BinItem struct {
	Bin   uint8
	BinID uint64
}

func (b *BinItem) Namespace() string {
	return "binID"
}

func (b *BinItem) ID() string {
	return string(b.Bin)
}

func (c *BinItem) String() string {
	return path.Join(c.Namespace(), c.ID())
}

func (b *BinItem) Clone() storage.Item {
	if b == nil {
		return nil
	}
	return &BinItem{
		Bin:   b.Bin,
		BinID: b.BinID,
	}
}

const binItemSize = 8

func (c *BinItem) Marshal() ([]byte, error) {
	buf := make([]byte, binItemSize)
	binary.BigEndian.PutUint64(buf, c.BinID)
	return buf, nil
}

func (c *BinItem) Unmarshal(buf []byte) error {
	if len(buf) != binItemSize {
		return errUnmarshalInvalidSize
	}
	c.BinID = binary.BigEndian.Uint64(buf)
	return nil
}

// EpochItem stores the timestamp in seconds of the initial creation of the reserve.
type EpochItem struct {
	Timestamp uint64
}

func (e *EpochItem) Namespace() string   { return "epochItem" }
func (e *EpochItem) ID() string          { return "" }
func (e *EpochItem) String() string      { return e.Namespace() }
func (e *EpochItem) Clone() storage.Item { return &EpochItem{e.Timestamp} }

const epochItemSize = 8

func (e *EpochItem) Marshal() ([]byte, error) {
	buf := make([]byte, epochItemSize)
	binary.BigEndian.PutUint64(buf, e.Timestamp)
	return buf, nil
}

func (e *EpochItem) Unmarshal(buf []byte) error {
	if len(buf) != epochItemSize {
		return errUnmarshalInvalidSize
	}
	e.Timestamp = binary.BigEndian.Uint64(buf)
	return nil
}

// radiusItem stores the current storage radius of the reserve.
type radiusItem struct {
	Radius uint8
}

func (r *radiusItem) Namespace() string {
	return "radius"
}

func (r *radiusItem) ID() string {
	return ""
}

func (r *radiusItem) String() string {
	return r.Namespace()
}

func (r *radiusItem) Clone() storage.Item {
	if r == nil {
		return nil
	}
	return &radiusItem{
		Radius: r.Radius,
	}
}

func (r *radiusItem) Marshal() ([]byte, error) {
	return []byte{r.Radius}, nil
}

func (r *radiusItem) Unmarshal(buf []byte) error {
	if len(buf) != 1 {
		return errUnmarshalInvalidSize
	}
	r.Radius = buf[0]
	return nil
}

func copyBytes(src []byte) []byte {
	if src == nil {
		return nil
	}
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}
