// Copyright 2023 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pullsync provides the pullsync protocol
// implementation.

package reserve

import (
	"encoding/binary"

	storage "github.com/ethersphere/bee/pkg/storagev2"
	"github.com/ethersphere/bee/pkg/swarm"
)

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
	return batchBinToString(b.Bin, b.BatchID) + b.Address.ByteString()
}

func batchBinToString(bin uint8, batchID []byte) string {
	return string(bin) + string(batchID)
}

func (b *batchRadiusItem) Clone() storage.Item {
	return &batchRadiusItem{
		Bin:     b.Bin,
		BatchID: b.BatchID,
		Address: b.Address,
		BinID:   b.BinID,
	}
}

func (b *batchRadiusItem) String() string {
	return ""
}

const batchRadiusItemSize = 1 + swarm.HashSize + swarm.HashSize + 8

func (b *batchRadiusItem) Marshal() ([]byte, error) {

	buf := make([]byte, batchRadiusItemSize)

	i := 0

	buf[i] = b.Bin
	i += 1

	copy(buf[i:], b.BatchID)
	i += swarm.HashSize

	copy(buf[i:], b.Address.Bytes())
	i += swarm.HashSize

	binary.BigEndian.PutUint64(buf[i:], b.BinID)

	return buf, nil
}

func (b *batchRadiusItem) Unmarshal(buf []byte) error {

	i := 0
	b.Bin = buf[i]
	i += 1

	b.BatchID = buf[i : i+swarm.HashSize]
	i += swarm.HashSize

	b.Address = swarm.NewAddress(buf[i : i+swarm.HashSize])
	i += swarm.HashSize

	b.BinID = binary.BigEndian.Uint64(buf[i : i+8])

	return nil
}

type chunkBinItem struct {
	Bin     uint8
	BinID   uint64
	Address swarm.Address
}

// bin
func (c *chunkBinItem) Namespace() string {
	return "chunkBin" + string(c.Bin)
}

// binID
func (c *chunkBinItem) ID() string {
	return binIDToString(c.BinID)
}

func binIDToString(binID uint64) string {
	binIDBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(binIDBytes, binID)
	return string(binIDBytes)
}

func (b *chunkBinItem) Clone() storage.Item {
	return &chunkBinItem{
		Bin:     b.Bin,
		BinID:   b.BinID,
		Address: b.Address,
	}
}

func (b *chunkBinItem) String() string {
	return ""
}

const chunkBinItemSize = 1 + 8 + swarm.HashSize

func (b *chunkBinItem) Marshal() ([]byte, error) {

	buf := make([]byte, chunkBinItemSize)
	i := 0

	buf[i] = b.Bin
	i += 1

	binary.BigEndian.PutUint64(buf[i:], b.BinID)
	i += 8

	copy(buf[i:], b.Address.Bytes())

	return buf, nil
}

func (b *chunkBinItem) Unmarshal(buf []byte) error {

	i := 0
	b.Bin = buf[i]
	i += 1

	b.BinID = binary.BigEndian.Uint64(buf[i : i+8])
	i += 8

	b.Address = swarm.NewAddress(buf[i : i+swarm.HashSize])

	return nil
}

type binItem struct {
	PO    uint8
	BinID uint64
}

func (b *binItem) Namespace() string {
	return "binID"
}

func (c *binItem) ID() string {
	return string(c.PO)
}

func (b *binItem) Clone() storage.Item {
	return &binItem{
		PO:    b.PO,
		BinID: b.BinID,
	}
}

func (b *binItem) String() string {
	return ""
}

const binItemSize = 8

func (b *binItem) Marshal() ([]byte, error) {
	buf := make([]byte, binItemSize)
	binary.BigEndian.PutUint64(buf, b.BinID)
	return buf, nil
}

func (b *binItem) Unmarshal(buf []byte) error {
	b.BinID = binary.BigEndian.Uint64(buf)
	return nil
}
