// Copyright 2026 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package stampcarrier implements the stamp carrier payload format and the
// carrier erasure-coded group. A stamp carrier is an ordinary CAC whose
// payload packs the postage stamps of a parent chunk's children so that the
// original stamp of an erasure-reconstructed chunk stays recoverable.
//
// Payload layout (zero-padded to swarm.ChunkSize):
//
//	header: count(2, BE) | batchID(32)                                  = 34 B
//	entry:  childIndex(2, BE) | index(8) | timestamp(8) | signature(65) = 83 B
//
// The stamp for child slot i lives in carrier i/MaxEntries; entries are
// sorted by ascending childIndex.
package stampcarrier

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/ethersphere/bee/v2/pkg/postage"
	"github.com/ethersphere/bee/v2/pkg/swarm"
	"github.com/klauspost/reedsolomon"
)

const (
	// HeaderSize is count(2, BE) plus batchID(32).
	HeaderSize = 34
	// EntrySize is childIndex(2, BE) plus the stamp without its batch ID.
	EntrySize = 2 + postage.StampSize - swarm.HashSize
	// MaxEntries is the number of entries that fit into one carrier payload.
	MaxEntries = (swarm.ChunkSize - HeaderSize) / EntrySize
	// GroupParities is the number of parity members protecting a carrier group.
	GroupParities = 2
)

var (
	// ErrPayloadSize is returned when a carrier payload has an unexpected length.
	ErrPayloadSize = errors.New("stampcarrier: invalid payload size")
	// ErrEntryCount is returned when a carrier payload advertises too many entries.
	ErrEntryCount = errors.New("stampcarrier: entry count out of range")
	// ErrGroupSize is returned when a carrier group has an unexpected member count.
	ErrGroupSize = errors.New("stampcarrier: invalid group size")
)

// Count returns the number of carrier chunks needed for the stamps of n children.
func Count(children int) int {
	if children <= 0 {
		return 0
	}
	return (children + MaxEntries - 1) / MaxEntries
}

// Pack packs the marshaled stamps of a parent's children into carrier
// payloads. stamps[i] is the 113-byte stamp of the child at slot i in
// [data ‖ parity], or nil when unknown; nil stamps are omitted from the
// payloads but still consume their slot range. The batch ID of the first
// non-nil stamp is hoisted into every header (single-batch uploads, spec §4);
// stamps of a different batch are skipped.
func Pack(stamps [][]byte) ([][]byte, error) {
	c := Count(len(stamps))
	if c == 0 {
		return nil, errors.New("stampcarrier: no children")
	}
	var batchID []byte
	for i, s := range stamps {
		if s != nil {
			if len(s) != postage.StampSize {
				return nil, fmt.Errorf("stampcarrier: stamp at slot %d has size %d", i, len(s))
			}
			batchID = s[:swarm.HashSize]
			break
		}
	}
	payloads := make([][]byte, c)
	for j := range payloads {
		p := make([]byte, swarm.ChunkSize)
		copy(p[2:HeaderSize], batchID)
		cnt := 0
		hi := min((j+1)*MaxEntries, len(stamps))
		for i := j * MaxEntries; i < hi; i++ {
			s := stamps[i]
			if s == nil {
				continue
			}
			if len(s) != postage.StampSize {
				return nil, fmt.Errorf("stampcarrier: stamp at slot %d has size %d", i, len(s))
			}
			if !bytes.Equal(s[:swarm.HashSize], batchID) {
				continue
			}
			off := HeaderSize + cnt*EntrySize
			binary.BigEndian.PutUint16(p[off:off+2], uint16(i))
			copy(p[off+2:off+EntrySize], s[swarm.HashSize:])
			cnt++
		}
		binary.BigEndian.PutUint16(p[0:2], uint16(cnt))
		payloads[j] = p
	}
	return payloads, nil
}

// Unpack parses one carrier payload and returns the reassembled full stamps
// keyed by child slot.
func Unpack(payload []byte) (map[uint16][]byte, error) {
	if len(payload) != swarm.ChunkSize {
		return nil, ErrPayloadSize
	}
	cnt := int(binary.BigEndian.Uint16(payload[0:2]))
	if cnt > MaxEntries {
		return nil, ErrEntryCount
	}
	batchID := payload[2:HeaderSize]
	entries := make(map[uint16][]byte, cnt)
	for e := range cnt {
		off := HeaderSize + e*EntrySize
		idx := binary.BigEndian.Uint16(payload[off : off+2])
		stamp := make([]byte, postage.StampSize)
		copy(stamp, batchID)
		copy(stamp[swarm.HashSize:], payload[off+2:off+EntrySize])
		entries[idx] = stamp
	}
	return entries, nil
}

// EncodeGroup erasure codes the carrier payloads with a (c, GroupParities)
// scheme and returns the parity payloads.
func EncodeGroup(payloads [][]byte) ([][]byte, error) {
	enc, err := reedsolomon.New(len(payloads), GroupParities)
	if err != nil {
		return nil, err
	}
	shards := make([][]byte, len(payloads)+GroupParities)
	copy(shards, payloads)
	for i := len(payloads); i < len(shards); i++ {
		shards[i] = make([]byte, swarm.ChunkSize)
	}
	if err := enc.Encode(shards); err != nil {
		return nil, err
	}
	return shards[len(payloads):], nil
}

// ReconstructGroup fills in nil group members in place. shards holds the c
// carrier payloads followed by the GroupParities parity payloads; any c
// non-nil members suffice.
func ReconstructGroup(shards [][]byte, c int) error {
	if len(shards) != c+GroupParities {
		return ErrGroupSize
	}
	enc, err := reedsolomon.New(c, GroupParities)
	if err != nil {
		return err
	}
	return enc.Reconstruct(shards)
}
