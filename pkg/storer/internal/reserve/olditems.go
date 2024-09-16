// Copyright 2024 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reserve

import (
	"encoding/binary"
	"path"

	"github.com/ethersphere/bee/v2/pkg/storage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
)

// BatchRadiusItemV1 allows iteration of the chunks with respect to bin and batchID.
// Used for batch evictions of certain bins.
type BatchRadiusItemV1 struct {
	Bin     uint8
	BatchID []byte
	Address swarm.Address
	BinID   uint64
}

func (b *BatchRadiusItemV1) Namespace() string {
	return "batchRadius"
}

func (b *BatchRadiusItemV1) ID() string {
	return string(b.BatchID) + string(b.Bin) + b.Address.ByteString()
}

func (b *BatchRadiusItemV1) String() string {
	return path.Join(b.Namespace(), b.ID())
}

func (b *BatchRadiusItemV1) Clone() storage.Item {
	if b == nil {
		return nil
	}
	return &BatchRadiusItemV1{
		Bin:     b.Bin,
		BatchID: copyBytes(b.BatchID),
		Address: b.Address.Clone(),
		BinID:   b.BinID,
	}
}

const batchRadiusItemSizeV1 = 1 + swarm.HashSize + swarm.HashSize + 8

func (b *BatchRadiusItemV1) Marshal() ([]byte, error) {

	if b.Address.IsZero() {
		return nil, errMarshalInvalidAddress
	}

	buf := make([]byte, batchRadiusItemSizeV1)

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

func (b *BatchRadiusItemV1) Unmarshal(buf []byte) error {

	if len(buf) != batchRadiusItemSizeV1 {
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

// ChunkBinItemV1 allows for iterating on ranges of bin and binIDs for chunks.
// BinIDs come in handy when syncing the reserve contents with other peers.
type ChunkBinItemV1 struct {
	Bin       uint8
	BinID     uint64
	Address   swarm.Address
	BatchID   []byte
	ChunkType swarm.ChunkType
}

func (c *ChunkBinItemV1) Namespace() string {
	return "chunkBin"
}

func (c *ChunkBinItemV1) ID() string {
	return binIDToString(c.Bin, c.BinID)
}

func (c *ChunkBinItemV1) String() string {
	return path.Join(c.Namespace(), c.ID())
}

func (c *ChunkBinItemV1) Clone() storage.Item {
	if c == nil {
		return nil
	}
	return &ChunkBinItemV1{
		Bin:       c.Bin,
		BinID:     c.BinID,
		Address:   c.Address.Clone(),
		BatchID:   copyBytes(c.BatchID),
		ChunkType: c.ChunkType,
	}
}

const chunkBinItemSizeV1 = 1 + 8 + swarm.HashSize + swarm.HashSize + 1

func (c *ChunkBinItemV1) Marshal() ([]byte, error) {

	if c.Address.IsZero() {
		return nil, errMarshalInvalidAddress
	}

	buf := make([]byte, chunkBinItemSizeV1)
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

	return buf, nil
}

func (c *ChunkBinItemV1) Unmarshal(buf []byte) error {

	if len(buf) != chunkBinItemSizeV1 {
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

	return nil
}
