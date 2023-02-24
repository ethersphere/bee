// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve

import (
	"encoding/binary"
	"errors"
	"path"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
)

var errMarshalInvalidAddress = errors.New("marshal: invalid address")
var errUnmarshalInvalidSize = errors.New("unmarshal: invalid size")

type batchRadiusItem struct {
	Bin     uint8
	BatchID []byte
	Address swarm.Address
	BinID   uint64
}

func (b *batchRadiusItem) Namespace() string {
	return "batchRadius"
}

// bin/batchID/ChunkAddr
func (b *batchRadiusItem) ID() string {
	return path.Join(batchBinToString(b.Bin, b.BatchID), b.Address.ByteString())
}

func batchBinToString(bin uint8, batchID []byte) string {
	return path.Join(string(bin), string(batchID))
}

func (b *batchRadiusItem) String() string {
	return path.Join(b.Namespace(), b.ID())
}

func (b *batchRadiusItem) Clone() storage.Item {
	if b == nil {
		return nil
	}
	return &batchRadiusItem{
		Bin:     b.Bin,
		BatchID: copyBytes(b.BatchID),
		Address: b.Address.Clone(),
		BinID:   b.BinID,
	}
}

const batchRadiusItemSize = 1 + swarm.HashSize + swarm.HashSize + 8

func (b *batchRadiusItem) Marshal() ([]byte, error) {

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

	return buf, nil
}

func (b *batchRadiusItem) Unmarshal(buf []byte) error {

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

	return nil
}

type chunkBinItem struct {
	Bin     uint8
	BinID   uint64
	Address swarm.Address
}

func (c *chunkBinItem) Namespace() string {
	return "chunkBin"
}

// bin/binID
func (c *chunkBinItem) ID() string {
	return binIDToString(c.Bin, c.BinID)
}

func binIDToString(bin uint8, binID uint64) string {
	binIDBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(binIDBytes, binID)
	return path.Join(string(bin), string(binIDBytes))
}

func (c *chunkBinItem) String() string {
	return path.Join(c.Namespace(), c.ID())
}

func (c *chunkBinItem) Clone() storage.Item {
	if c == nil {
		return nil
	}
	return &chunkBinItem{
		Bin:     c.Bin,
		BinID:   c.BinID,
		Address: c.Address.Clone(),
	}
}

const chunkBinItemSize = 1 + 8 + swarm.HashSize

func (c *chunkBinItem) Marshal() ([]byte, error) {

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

	return buf, nil
}

func (c *chunkBinItem) Unmarshal(buf []byte) error {

	if len(buf) != chunkBinItemSize {
		return errUnmarshalInvalidSize
	}

	i := 0
	c.Bin = buf[i]
	i += 1

	c.BinID = binary.BigEndian.Uint64(buf[i : i+8])
	i += 8

	c.Address = swarm.NewAddress(buf[i : i+swarm.HashSize]).Clone()

	return nil
}

type binItem struct {
	Bin   uint8
	BinID uint64
}

func (b *binItem) Namespace() string {
	return "binID"
}

func (b *binItem) ID() string {
	return string(b.Bin)
}

func (c *binItem) String() string {
	return path.Join(c.Namespace(), c.ID())
}
func (b *binItem) Clone() storage.Item {
	if b == nil {
		return nil
	}
	return &binItem{
		Bin:   b.Bin,
		BinID: b.BinID,
	}
}

const binItemSize = 8

func (c *binItem) Marshal() ([]byte, error) {
	buf := make([]byte, binItemSize)
	binary.BigEndian.PutUint64(buf, c.BinID)
	return buf, nil
}

func (c *binItem) Unmarshal(buf []byte) error {
	if len(buf) != binItemSize {
		return errUnmarshalInvalidSize
	}
	c.BinID = binary.BigEndian.Uint64(buf)
	return nil
}

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
