// Copyright 2020 The Swarm Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package postage

import (
	"encoding/binary"
	"math/big"
)

// Batch represents a postage batch, a payment on the blockchain.
type Batch struct {
	ID     []byte   // batch ID
	Value  *big.Int // normalised balance of the batch
	Start  uint64   // block number the batch was created
	Owner  []byte   // owner's ethereum address
	Depth  uint8    // batch depth, i.e., size = 2^{depth}
	Radius uint8    // reserve radius, non-serialised
}

// MarshalBinary implements BinaryMarshaller. It will attempt to serialize the
// postage batch to a byte slice.
// serialised as ID(32)|big endian value(32)|start block(8)|owner addr(20)|depth(1)
func (b *Batch) MarshalBinary() ([]byte, error) {
	out := make([]byte, 94)
	copy(out, b.ID)
	value := b.Value.Bytes()
	copy(out[64-len(value):], value)
	binary.BigEndian.PutUint64(out[64:72], b.Start)
	copy(out[72:], b.Owner)
	out[92] = b.Depth
	out[93] = b.Radius
	return out, nil
}

// UnmarshalBinary implements BinaryUnmarshaller. It will attempt deserialize
// the given byte slice into the batch.
func (b *Batch) UnmarshalBinary(buf []byte) error {
	b.ID = buf[:32]
	b.Value = big.NewInt(0).SetBytes(buf[32:64])
	b.Start = binary.BigEndian.Uint64(buf[64:72])
	b.Owner = buf[72:92]
	b.Depth = buf[92]
	b.Radius = buf[93]
	return nil
}
